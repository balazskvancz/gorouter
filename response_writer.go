package gorouter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type Response interface {
	Encode(w io.Writer) error
	ContentType() string
}

type DefaultResponse struct {
	Data any
}

func (dr *DefaultResponse) Encode(w io.Writer) error {
	b, ok := dr.Data.([]byte)
	if !ok {
		return errors.New("cant assert to []byte")
	}
	_, err := w.Write(b)
	return err
}

func (dr *DefaultResponse) ContentType() string {
	return "text/plain"
}

type JsonResponse struct {
	Data any
}

func (js *JsonResponse) Encode(w io.Writer) error {
	return json.NewEncoder(w).Encode(js.Data)
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

	w http.ResponseWriter
}

func (rw *responseWriter) Empty() {
	rw.buff.Reset()
	rw.statusCode = 0
	rw.w = nil
}

func (rw *responseWriter) write(b []byte) (int, error) {
	return rw.buff.Write(b)
}

func (rw *responseWriter) render(r Response) error {
	rw.addHeader(contentTypeHeaderKey, r.ContentType())
	return r.Encode(rw.buff)
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
	_, err := io.Copy(rw.buff, r)
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
