package gorouter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type Response interface {
	Encode(w io.Writer) (int, error)
	ContentType() string
}

type DefaultResponse struct {
	Data any
}

func (dr *DefaultResponse) Encode(w io.Writer) (int, error) {
	b, ok := dr.Data.([]byte)
	if !ok {
		return 0, errors.New("cant assert to []byte")
	}
	return w.Write(b)
}

func (dr *DefaultResponse) ContentType() string {
	return "text/plain"
}

type JsonResponse struct {
	Data any
}

type countWriter struct {
	w            io.Writer
	writtenBytes int
}

func (cw *countWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.writtenBytes = n
	return n, err
}

func (js *JsonResponse) Encode(w io.Writer) (int, error) {
	cw := &countWriter{w: w}
	if err := json.NewEncoder(cw).Encode(js.Data); err != nil {
		return 0, err
	}
	return cw.writtenBytes, nil
}

func (js *JsonResponse) ContentType() string {
	return "application/json"
}

var (
	_ (Response) = (*DefaultResponse)(nil)
	_ (Response) = (*JsonResponse)(nil)
)

type responseWriter struct {
	defaultStatusCode int
	statusCode        int
	buff              *bytes.Buffer
	writtenBytes      int

	w http.ResponseWriter
}

func (rw *responseWriter) Empty() {
	rw.buff.Reset()
	rw.statusCode = 0
	rw.w = nil
	rw.writtenBytes = 0
}

func (rw *responseWriter) write(b []byte) (int, error) {
	n, err := rw.buff.Write(b)
	rw.writtenBytes += n
	return n, err
}

func (rw *responseWriter) render(r Response) (int, error) {
	rw.addHeader(contentTypeHeaderKey, r.ContentType())
	n, err := r.Encode(rw.buff)
	rw.writtenBytes += n
	return n, err
}

func (rw *responseWriter) setStatus(statusCode int) {
	rw.statusCode = statusCode
}

func (rw *responseWriter) addHeader(key, value string) {
	rw.w.Header().Add(key, value)
}

func (rw *responseWriter) copy(r io.Reader) error {
	if rw == nil {
		return errors.New("response writer is <nil>")
	}
	n, err := io.Copy(rw.buff, r)
	rw.writtenBytes += int(n)
	return err
}

func (rw *responseWriter) flush() {
	statusCode := defaultStatusCode
	if rw.defaultStatusCode > 0 {
		statusCode = rw.defaultStatusCode
	}
	if rw.statusCode > 0 {
		statusCode = rw.statusCode
	}

	rw.w.WriteHeader(statusCode)
	rw.buff.WriteTo(rw.w)
}
