package slogecho

import (
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
