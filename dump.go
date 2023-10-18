package slogecho

import (
	"bytes"
	"net/http"
)

type bodyWriter struct {
	http.ResponseWriter
	body    *bytes.Buffer
	maxSize int
}

// implements http.ResponseWriter
func (w bodyWriter) Write(b []byte) (int, error) {
	if w.body.Len()+len(b) > w.maxSize {
		w.body.Write(b[:w.maxSize-w.body.Len()])
	} else {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func newBodyWriter(writer http.ResponseWriter, maxSize int) *bodyWriter {
	return &bodyWriter{
		body:           bytes.NewBufferString(""),
		ResponseWriter: writer,
		maxSize:        maxSize,
	}
}
