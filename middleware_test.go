package slogecho

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// The structured "error" log attribute must appear for every non-nil error, with the error's
// own code: HTTPStatusCoder types other than *echo.HTTPError (Echo v5 router sentinels, custom
// errors) keep their code instead of a hardcoded 500, and plain errors fall back to 500.
func TestMiddlewareStructuredErrorAttributeUsesErrorCode(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantCode    int
		wantMessage string
	}{
		{"real HTTPError", echo.NewHTTPError(http.StatusBadRequest, "bad input"), http.StatusBadRequest, "bad input"},
		{"custom HTTPStatusCoder", httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"}, http.StatusTooManyRequests, "please slow down"},
		{"plain error", errors.New("boom"), http.StatusInternalServerError, "boom"},
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

			errAttr, ok := logLine["error"].(map[string]any)
			if !ok {
				t.Fatalf("missing structured \"error\" log attribute (log: %s)", logBuf.String())
			}
			if code, _ := errAttr["code"].(float64); int(code) != tt.wantCode {
				t.Fatalf("error.code = %v, want %d (a hardcoded 500 here would hide the real status)", errAttr["code"], tt.wantCode)
			}
			if msg, _ := errAttr["message"].(string); msg != tt.wantMessage {
				t.Fatalf("error.message = %q, want %q", msg, tt.wantMessage)
			}
		})
	}
}

// serveAndDecodeLog serves one GET request through the middleware and returns the decoded JSON
// log line.
func serveAndDecodeLog(t *testing.T, handler echo.HandlerFunc) map[string]any {
	t.Helper()

	var logBuf bytes.Buffer
	e := echo.New()
	e.Use(New(slog.New(slog.NewJSONHandler(&logBuf, nil))))
	e.GET("/x", handler)

	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	var logLine map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &logLine); err != nil {
		t.Fatalf("failed to parse log line: %v (line: %s)", err, logBuf.String())
	}
	return logLine
}

// Once the handler has committed a response, echo.ResolveResponseStatus returns the committed
// status and ignores the error. error.code must still describe the error itself, not the wire
// status.
func TestMiddlewareErrorCodeAfterCommittedResponse(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"HTTPError", echo.NewHTTPError(http.StatusBadRequest, "bad"), http.StatusBadRequest},
		{"plain error", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLine := serveAndDecodeLog(t, func(c *echo.Context) error {
				if err := c.String(http.StatusOK, "ok"); err != nil {
					return err
				}
				return tt.err
			})

			errAttr, ok := logLine["error"].(map[string]any)
			if !ok {
				t.Fatalf("missing structured \"error\" log attribute (log: %v)", logLine)
			}
			if code, _ := errAttr["code"].(float64); int(code) != tt.wantCode {
				t.Fatalf("error.code = %v, want %d", errAttr["code"], tt.wantCode)
			}
		})
	}
}

// Any error that is not an *echo.HTTPError has no separate wrapped cause, so the error itself
// is the internal cause: it must be exposed both as the top-level "internal" attribute and as
// error.internal.
func TestMiddlewareInternalAttributeForNonHTTPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"plain error", errors.New("boom")},
		{"wrapped error", fmt.Errorf("load user: %w", errors.New("db down"))},
		{"custom HTTPStatusCoder", httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLine := serveAndDecodeLog(t, func(c *echo.Context) error { return tt.err })

			if internal, _ := logLine["internal"].(string); internal != tt.err.Error() {
				t.Fatalf("internal = %q, want %q (log: %v)", internal, tt.err.Error(), logLine)
			}
			errAttr, _ := logLine["error"].(map[string]any)
			if _, ok := errAttr["internal"]; !ok {
				t.Fatalf("missing error.internal (log: %v)", logLine)
			}
		})
	}
}

// Log consumers rely on a stable schema: error.internal is always present, null when the
// *echo.HTTPError wraps no cause.
func TestMiddlewareErrorInternalKeyAlwaysPresent(t *testing.T) {
	logLine := serveAndDecodeLog(t, func(c *echo.Context) error {
		return echo.NewHTTPError(http.StatusBadRequest, "bad")
	})

	errAttr, ok := logLine["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing structured \"error\" log attribute (log: %v)", logLine)
	}
	if _, ok := errAttr["internal"]; !ok {
		t.Fatalf("missing error.internal key (log: %v)", logLine)
	}
}

// errorKind is one way a handler can return an error, with everything the middleware is
// expected to derive from it.
type errorKind struct {
	name         string
	err          error
	wantStatus   int    // wire status and log response.status
	wantBody     string // body written by Echo's default error handler
	wantLevel    string
	wantMsg      string // log message and error.message
	wantInternal any    // error.internal and top-level internal; nil when absent
}

// errorKinds covers every way a handler can return an error: a plain Go error, an
// *echo.HTTPError (with and without a wrapped cause), and an echo.HTTPStatusCoder that is not
// an *echo.HTTPError (custom type and Echo v5 router sentinel).
func errorKinds() []errorKind {
	return []errorKind{
		{
			name:         "plain error",
			err:          errors.New("boom"),
			wantStatus:   http.StatusInternalServerError,
			wantBody:     `{"message":"Internal Server Error"}`,
			wantLevel:    "ERROR",
			wantMsg:      "boom",
			wantInternal: "boom",
		},
		{
			name:         "HTTPError",
			err:          echo.NewHTTPError(http.StatusBadRequest, "bad input"),
			wantStatus:   http.StatusBadRequest,
			wantBody:     `{"message":"bad input"}`,
			wantLevel:    "WARN",
			wantMsg:      "bad input",
			wantInternal: nil,
		},
		{
			name:         "HTTPError with wrapped cause",
			err:          echo.NewHTTPError(http.StatusServiceUnavailable, "try later").Wrap(errors.New("db down")),
			wantStatus:   http.StatusServiceUnavailable,
			wantBody:     `{"message":"try later"}`,
			wantLevel:    "ERROR",
			wantMsg:      "try later",
			wantInternal: "db down",
		},
		{
			name:         "custom HTTPStatusCoder",
			err:          httpStatusCoderError{code: http.StatusTooManyRequests, msg: "please slow down"},
			wantStatus:   http.StatusTooManyRequests,
			wantBody:     `{"message":"Too Many Requests"}`,
			wantLevel:    "WARN",
			wantMsg:      "please slow down",
			wantInternal: "please slow down",
		},
		{
			name:         "Echo router sentinel HTTPStatusCoder",
			err:          echo.ErrNotFound,
			wantStatus:   http.StatusNotFound,
			wantBody:     `{"message":"Not Found"}`,
			wantLevel:    "WARN",
			wantMsg:      "Not Found",
			wantInternal: "Not Found",
		},
	}
}

// TestMiddlewareErrorKindsMatrix runs every middleware feature against every error kind.
func TestMiddlewareErrorKindsMatrix(t *testing.T) {
	for _, kind := range errorKinds() {
		t.Run(kind.name, func(t *testing.T) {
			t.Run("response", func(t *testing.T) {
				e := echo.New()
				e.Use(New(slog.New(slog.NewTextHandler(io.Discard, nil))))
				e.GET("/x", func(c *echo.Context) error { return kind.err })

				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

				if rec.Code != kind.wantStatus {
					t.Fatalf("status = %d, want %d", rec.Code, kind.wantStatus)
				}
				if got := rec.Body.String(); got != kind.wantBody+"\n" {
					t.Fatalf("body = %q, want %q", got, kind.wantBody+"\n")
				}
			})

			t.Run("log", func(t *testing.T) {
				logLine := serveAndDecodeLog(t, func(c *echo.Context) error { return kind.err })

				if level, _ := logLine["level"].(string); level != kind.wantLevel {
					t.Fatalf("level = %q, want %q", level, kind.wantLevel)
				}
				if msg, _ := logLine["msg"].(string); msg != kind.wantMsg {
					t.Fatalf("msg = %q, want %q", msg, kind.wantMsg)
				}
				response, _ := logLine["response"].(map[string]any)
				if status, _ := response["status"].(float64); int(status) != kind.wantStatus {
					t.Fatalf("response.status = %v, want %d", response["status"], kind.wantStatus)
				}

				errAttr, ok := logLine["error"].(map[string]any)
				if !ok {
					t.Fatalf("missing structured \"error\" log attribute (log: %v)", logLine)
				}
				if code, _ := errAttr["code"].(float64); int(code) != kind.wantStatus {
					t.Fatalf("error.code = %v, want %d", errAttr["code"], kind.wantStatus)
				}
				if msg, _ := errAttr["message"].(string); msg != kind.wantMsg {
					t.Fatalf("error.message = %q, want %q", msg, kind.wantMsg)
				}
				internal, hasInternal := errAttr["internal"]
				if !hasInternal {
					t.Fatalf("missing error.internal key (log: %v)", logLine)
				}
				if kind.wantInternal == nil {
					if internal != nil {
						t.Fatalf("error.internal = %v, want null", internal)
					}
					if _, ok := logLine["internal"]; ok {
						t.Fatalf("top-level internal = %v, want absent", logLine["internal"])
					}
				} else if logLine["internal"] != kind.wantInternal {
					t.Fatalf("top-level internal = %v, want %v", logLine["internal"], kind.wantInternal)
				}
			})

			t.Run("committed response keeps error.code", func(t *testing.T) {
				logLine := serveAndDecodeLog(t, func(c *echo.Context) error {
					if err := c.String(http.StatusOK, "ok"); err != nil {
						return err
					}
					return kind.err
				})

				response, _ := logLine["response"].(map[string]any)
				if status, _ := response["status"].(float64); int(status) != http.StatusOK {
					t.Fatalf("response.status = %v, want the committed %d", response["status"], http.StatusOK)
				}
				errAttr, _ := logLine["error"].(map[string]any)
				if code, _ := errAttr["code"].(float64); int(code) != kind.wantStatus {
					t.Fatalf("error.code = %v, want %d", errAttr["code"], kind.wantStatus)
				}
			})

			t.Run("filter receives original error and status", func(t *testing.T) {
				var gotErr error
				var gotStatus int
				config := DefaultConfig()
				config.Filters = []Filter{
					func(c *echo.Context, err error) bool {
						gotErr = err
						_, gotStatus = echo.ResolveResponseStatus(c.Response(), err)
						return true
					},
				}

				e := echo.New()
				e.Use(NewWithConfig(slog.New(slog.NewTextHandler(io.Discard, nil)), config))
				e.GET("/x", func(c *echo.Context) error { return kind.err })
				e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

				// Identity, not errors.Is: errors.Is would also accept kind.err wrapped in an
				// *echo.HTTPError.
				if gotErr != kind.err {
					t.Fatalf("Filter received %v, want %v unchanged", gotErr, kind.err)
				}
				if gotStatus != kind.wantStatus {
					t.Fatalf("status resolved inside Filter = %d, want %d", gotStatus, kind.wantStatus)
				}
			})

			t.Run("IgnoreStatus filter drops the log", func(t *testing.T) {
				var logBuf bytes.Buffer
				config := DefaultConfig()
				config.Filters = []Filter{IgnoreStatus(kind.wantStatus)}

				e := echo.New()
				e.Use(NewWithConfig(slog.New(slog.NewJSONHandler(&logBuf, nil)), config))
				e.GET("/x", func(c *echo.Context) error { return kind.err })
				e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

				if logBuf.Len() != 0 {
					t.Fatalf("IgnoreStatus(%d) kept the log line: %s", kind.wantStatus, logBuf.String())
				}
			})

			t.Run("custom message receives original error", func(t *testing.T) {
				var logBuf bytes.Buffer
				var gotErr error
				config := DefaultConfig()
				config.WithCustomMessage = func(c *echo.Context, err error) string {
					gotErr = err
					return "custom"
				}

				e := echo.New()
				e.Use(NewWithConfig(slog.New(slog.NewJSONHandler(&logBuf, nil)), config))
				e.GET("/x", func(c *echo.Context) error { return kind.err })
				e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

				if gotErr != kind.err {
					t.Fatalf("WithCustomMessage received %v, want %v unchanged", gotErr, kind.err)
				}
				var logLine map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &logLine); err != nil {
					t.Fatalf("failed to parse log line: %v (line: %s)", err, logBuf.String())
				}
				if msg, _ := logLine["msg"].(string); msg != "custom" {
					t.Fatalf("msg = %q, want %q", msg, "custom")
				}
			})
		})
	}
}
