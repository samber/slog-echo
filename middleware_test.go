package slogecho

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// Echo v5's router returns sentinel errors (echo.ErrNotFound, echo.ErrMethodNotAllowed, ...)
// that implement echo.HTTPStatusCoder but are not *echo.HTTPError. The middleware must not
// downgrade these to a 500, since it forwards its returned error to Echo's real error handler.
func TestMiddlewarePreservesRouterStatusCodes(t *testing.T) {
	e := echo.New()
	e.Use(New(slog.New(slog.NewTextHandler(io.Discard, nil))))
	e.GET("/exists", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.GET("/boom", func(c *echo.Context) error { return errors.New("boom") })

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"unmatched route", http.MethodGet, "/missing", http.StatusNotFound, `{"message":"Not Found"}`},
		{"wrong method", http.MethodPost, "/exists", http.StatusMethodNotAllowed, `{"message":"Method Not Allowed"}`},
		{"plain handler error", http.MethodGet, "/boom", http.StatusInternalServerError, `{"message":"Internal Server Error"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := rec.Body.String(); got != tt.wantBody+"\n" {
				t.Fatalf("body = %q, want %q", got, tt.wantBody+"\n")
			}
		})
	}
}

// httpStatusCoderError is a minimal echo.HTTPStatusCoder, standing in for any error type
// (not just Echo's own router sentinels) that carries its status this way instead of being
// an *echo.HTTPError.
type httpStatusCoderError struct {
	code int
	msg  string
}

func (e httpStatusCoderError) Error() string   { return e.msg }
func (e httpStatusCoderError) StatusCode() int { return e.code }

// A custom error type implementing echo.HTTPStatusCoder (not *echo.HTTPError) must keep its
// own status code instead of being downgraded to 500, the same guarantee the router sentinels
// above rely on.
func TestMiddlewarePreservesCustomHTTPStatusCoderError(t *testing.T) {
	var logBuf bytes.Buffer
	e := echo.New()
	e.Use(New(slog.New(slog.NewJSONHandler(&logBuf, nil))))

	wantErr := httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"}
	e.GET("/limited", func(c *echo.Context) error { return wantErr })

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}

	var logLine map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &logLine); err != nil {
		t.Fatalf("failed to parse log line: %v (line: %s)", err, logBuf.String())
	}
	if msg, _ := logLine["msg"].(string); msg != wantErr.Error() {
		t.Fatalf("log msg = %q, want %q", msg, wantErr.Error())
	}
	response, _ := logLine["response"].(map[string]any)
	if status, _ := response["status"].(float64); int(status) != http.StatusTooManyRequests {
		t.Fatalf("log response.status = %v, want %d", response["status"], http.StatusTooManyRequests)
	}
}

// A plain error returned by a handler (one that is neither *echo.HTTPError nor an
// echo.HTTPStatusCoder) used to be wrapped into a fresh 500 *echo.HTTPError, and that wrapped
// error was what the middleware returned too, replacing the handler's own error with a fixed
// "Internal Server Error" message. The log must show the handler's own message instead.
func TestMiddlewareForwardsPlainHandlerErrorUnchanged(t *testing.T) {
	var logBuf bytes.Buffer
	e := echo.New()
	e.Use(New(slog.New(slog.NewJSONHandler(&logBuf, nil))))

	wantErr := errors.New("my error")
	e.GET("/boom", func(c *echo.Context) error { return wantErr })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var logLine map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &logLine); err != nil {
		t.Fatalf("failed to parse log line: %v (line: %s)", err, logBuf.String())
	}
	if msg, _ := logLine["msg"].(string); msg != wantErr.Error() {
		t.Fatalf("log msg = %q, want %q", msg, wantErr.Error())
	}
	response, _ := logLine["response"].(map[string]any)
	if status, _ := response["status"].(float64); int(status) != http.StatusInternalServerError {
		t.Fatalf("log response.status = %v, want %d", response["status"], http.StatusInternalServerError)
	}
}

// config.Filters used to receive the wrapped 500 *echo.HTTPError for any non-HTTPError
// handler error, never the handler's own error. It must now see the same error the handler
// returned, at its real resolved status. This matters beyond identity: filters.go's
// AcceptStatus/IgnoreStatus family call echo.ResolveResponseStatus(c.Response(), err) on the
// exact error a Filter receives, so as long as the filter loop saw the forced-500 wrapper,
// AcceptStatus(404) (for example) could never match a real 404 handler error.
func TestMiddlewareFilterReceivesOriginalError(t *testing.T) {
	e := echo.New()
	wantErr := httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"}

	var gotErr error
	var gotStatus int
	config := DefaultConfig()
	config.Filters = []Filter{
		func(c *echo.Context, err error) bool {
			gotErr = err
			_, gotStatus = echo.ResolveResponseStatus(c.Response(), err)
			return true // don't skip logging
		},
	}
	e.Use(NewWithConfig(slog.New(slog.NewTextHandler(io.Discard, nil)), config))
	e.GET("/limited", func(c *echo.Context) error { return wantErr })

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("Filter received %v, want the original error %v unchanged", gotErr, wantErr)
	}
	var httpErr *echo.HTTPError
	if errors.As(gotErr, &httpErr) {
		t.Fatalf("Filter received a wrapped *echo.HTTPError (code %d), want the original error", httpErr.Code)
	}
	if gotStatus != http.StatusTooManyRequests {
		t.Fatalf("status resolved inside the Filter = %d, want %d (the forced-500 wrapper would have hidden this)", gotStatus, http.StatusTooManyRequests)
	}
}

// The built-in IgnoreStatus filter is the real-world consumer of the bug in the test above:
// it resolves its own status from the exact error the middleware passes to config.Filters. If
// that error were still forced to 500 (the pre-fix behavior), IgnoreStatus(404) could never
// tell a real 404 apart from any other handler error, and would drop every request's log.
func TestMiddlewareIgnoreStatusFilterMatchesRouterNotFound(t *testing.T) {
	var logBuf bytes.Buffer
	e := echo.New()
	config := DefaultConfig()
	config.Filters = []Filter{IgnoreStatus(http.StatusNotFound)}
	e.Use(NewWithConfig(slog.New(slog.NewJSONHandler(&logBuf, nil)), config))
	e.GET("/exists", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if logBuf.Len() != 0 {
		t.Fatalf("IgnoreStatus(404) should have dropped the log line, got: %s", logBuf.String())
	}
}

// config.WithCustomMessage used to receive the wrapped 500 *echo.HTTPError for any
// non-HTTPError handler error, never the handler's own error. It must now see the same error
// the handler returned.
func TestMiddlewareCustomMessageReceivesOriginalError(t *testing.T) {
	e := echo.New()
	wantErr := errors.New("boom")

	var gotErr error
	config := DefaultConfig()
	config.WithCustomMessage = func(c *echo.Context, err error) string {
		gotErr = err
		return "custom message"
	}
	e.Use(NewWithConfig(slog.New(slog.NewTextHandler(io.Discard, nil)), config))
	e.GET("/boom", func(c *echo.Context) error { return wantErr })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("WithCustomMessage received %v, want the original error %v unchanged", gotErr, wantErr)
	}
	var httpErr *echo.HTTPError
	if errors.As(gotErr, &httpErr) {
		t.Fatalf("WithCustomMessage received a wrapped *echo.HTTPError (code %d), want the original plain error", httpErr.Code)
	}
}

// The structured "error" log attribute (code/message/internal) is only built from an actual
// *echo.HTTPError. Before this fix every non-nil handler error was forced into one, so the
// attribute always appeared, with a misleading code (always 500, even for a real 404/405).
// Now it only appears for a genuine *echo.HTTPError; other status-carrying or plain errors
// rely on the top-level msg and response.status fields instead.
func TestMiddlewareStructuredErrorAttributeOnlyForHTTPError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantAttr bool
	}{
		{"real HTTPError", echo.NewHTTPError(http.StatusBadRequest, "bad input"), true},
		{"custom HTTPStatusCoder", httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"}, false},
		{"plain error", errors.New("boom"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			e := echo.New()
			e.Use(New(slog.New(slog.NewJSONHandler(&logBuf, nil))))
			e.GET("/x", func(c *echo.Context) error { return tt.err })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			var logLine map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &logLine); err != nil {
				t.Fatalf("failed to parse log line: %v (line: %s)", err, logBuf.String())
			}
			_, hasErrorAttr := logLine["error"]
			if hasErrorAttr != tt.wantAttr {
				t.Fatalf("structured \"error\" log attribute present = %v, want %v (log: %s)", hasErrorAttr, tt.wantAttr, logBuf.String())
			}
		})
	}
}
