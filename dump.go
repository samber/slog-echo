package slogecho

import (
	"bytes"
	"net/http"
)

type bodyWriter struct {
	http.ResponseWriter
	body *bytes.Buffer
}

// implements http.ResponseWriter
func (w bodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func newBodyWriter(writer http.ResponseWriter) *bodyWriter {
	return &bodyWriter{
		body:           bytes.NewBufferString(""),
		ResponseWriter: writer,
	}
}
