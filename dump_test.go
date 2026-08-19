package slogecho

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestBodyWriterUnwrapResolvesTheRealStatus(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("POST", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	wrapped := newBodyWriter(c.Response(), ResponseBodyMaxSize, false)
	c.SetResponse(wrapped)
	wrapped.WriteHeader(202)

	if _, status := echo.ResolveResponseStatus(c.Response(), nil); status != 202 {
		t.Fatalf("resolved status = %d, want the 202 the handler wrote", status)
	}
}
