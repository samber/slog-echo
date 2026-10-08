package slogecho

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestBodyWriterCapturesResponsePrefix(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		chunks []string
	}{
		{"below limit", 10, []string{"abc", "def"}},
		{"exact limit", 10, []string{"abcdefgh", "ij"}},
		{"cross limit", 10, []string{"abcdefgh", "ijkl"}},
		{"write after limit", 10, []string{"abcdefghij", "kl", "m"}},
		{"empty write after limit", 10, []string{"abcdefghij", "", "k"}},
		{"empty write near limit", 10, []string{"abcdefgh", "", "ijkl"}},
		{"large writes", 10, []string{"abcdefghijkl", "mnopqrstuv"}},
		{"zero limit", 0, []string{"abc", "", "def"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			w := newBodyWriter(rec, tt.limit, true)
			var response strings.Builder
			for _, chunk := range tt.chunks {
				n, err := w.Write([]byte(chunk))
				if err != nil || n != len(chunk) {
					t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(chunk))
				}
				response.WriteString(chunk)
				want := response.String()[:min(tt.limit, response.Len())]
				if got := w.body.String(); got != want {
					t.Errorf("captured body = %q, want %q", got, want)
				}
				if got := rec.Body.String(); got != response.String() {
					t.Fatalf("response body = %q, want %q", got, response.String())
				}
				if w.bytes != response.Len() {
					t.Fatalf("response length = %d, want %d", w.bytes, response.Len())
				}
			}
		})
	}
}

func TestBodyWriterReadFromCapturesResponsePrefix(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newBodyWriter(rec, 10, true)
	reader := io.MultiReader(strings.NewReader("abcdefgh"), strings.NewReader("ijkl"), strings.NewReader("mn"))
	n, err := w.ReadFrom(reader)
	if err != nil || n != 14 {
		t.Fatalf("ReadFrom = (%d, %v), want (14, nil)", n, err)
	}
	if got := w.body.String(); got != "abcdefghij" {
		t.Errorf("captured body = %q, want %q", got, "abcdefghij")
	}
	if got := rec.Body.String(); got != "abcdefghijklmn" || w.bytes != 14 {
		t.Errorf("response = %q, length = %d", got, w.bytes)
	}
}

func TestBodyWriterWithoutCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newBodyWriter(rec, 3, false)
	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ReadFrom(bytes.NewBufferString("efgh")); err != nil {
		t.Fatal(err)
	}
	if w.body != nil || rec.Body.String() != "abcdefgh" || w.bytes != 8 {
		t.Errorf("body = %v, response = %q, length = %d", w.body, rec.Body.String(), w.bytes)
	}
}

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
