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

// A plain error returned by a handler (one that is neither *echo.HTTPError nor an
// echo.HTTPStatusCoder) must come back out of the middleware unchanged: it only used to wrap
// it into a fresh 500 *echo.HTTPError for logging purposes, but returned that wrapped error
// too, replacing whatever the handler actually returned. Calling the middleware function
// directly (instead of going through Echo.ServeHTTP + HTTPErrorHandler) isolates that
// contract: nothing downstream of the middleware's own return value is involved.
func TestMiddlewareForwardsPlainHandlerErrorUnchanged(t *testing.T) {
	var logBuf bytes.Buffer
	e := echo.New()
	mw := New(slog.New(slog.NewJSONHandler(&logBuf, nil)))

	wantErr := errors.New("my error")
	handler := mw(func(c *echo.Context) error { return wantErr })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	gotErr := handler(c)

	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("middleware returned %v, want the original error %v unchanged", gotErr, wantErr)
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
