package gorouter

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type httpVersion = string

const (
	version10 httpVersion = "HTTP/1.0"
	version11 httpVersion = "HTTP/1.1"

	asciOffset int = 48
)

type responseWriter struct {
	httpVersion       httpVersion
	defaultStatusCode int
	statusCode        int
	header            http.Header
	buff              *bytes.Buffer
	w                 *bufio.ReadWriter
}

var crlf []byte = []byte("\r\n")

func newResponseWriter(statusCode int) *responseWriter {
	return &responseWriter{
		defaultStatusCode: statusCode,
		header:            http.Header{},
		buff:              &bytes.Buffer{},
	}
}

func (rw *responseWriter) Empty() {
	rw.buff.Reset()
	rw.header = http.Header{}
	rw.statusCode = 0
	rw.w = nil
}

func (rw *responseWriter) setStatus(statusCode int) {
	rw.statusCode = statusCode
}

func (rw *responseWriter) setVersion(version httpVersion) {
	rw.httpVersion = version
}

func (rw *responseWriter) addHeader(key, value string) {
	rw.header.Add(key, value)
}

func (rw *responseWriter) write(b []byte) {
	rw.buff.Write(b)
}

func (rw *responseWriter) copy(r io.Reader) {
	if rw == nil {
		return
	}
	if _, err := io.Copy(rw.buff, r); err != nil {
		fmt.Println(err)
	}
}

func (rw *responseWriter) flush() error {
	response := rw.createResponseHeaders()

	if rw.buff != nil {
		rw.buff.WriteTo(response)
	}

	if _, err := response.WriteTo(rw.w); err != nil {
		return err
	}

	// Maybe a rw.buff.Reset() should be called here.

	return rw.w.Flush()
}

func createStatusLine(httpVersion httpVersion, statusCode int) *bytes.Buffer {
	var (
		statusText = http.StatusText(statusCode)
		buff       = &bytes.Buffer{}
	)

	var (
		firstDigit  = (statusCode / 100) + asciOffset
		secondDigit = ((statusCode / 10) % 10) + asciOffset
		thirdDigit  = (statusCode % 10) + asciOffset
	)

	buff.Write([]byte(httpVersion))
	buff.WriteRune(' ')
	buff.Write([]byte{byte(firstDigit), byte(secondDigit), byte(thirdDigit)})
	buff.WriteRune(' ')
	buff.Write([]byte(statusText))
	buff.Write(crlf)

	return buff
}

func (rw *responseWriter) createResponseHeaders() *bytes.Buffer {
	b := createStatusLine(rw.httpVersion, rw.statusCode)

	for k, v := range rw.header {
		b.WriteString(fmt.Sprintf("%s: %s", k, strings.Join(v, ";")))
		b.Write(crlf)
	}

	if val := rw.header.Get("Date"); val == "" {
		b.Write([]byte("Date: "))
		b.WriteString(time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
		b.Write(crlf)
	}

	b.Write(crlf)

	return b
}
