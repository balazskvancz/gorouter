package gorouter

import (
	"bytes"
	ctxpkg "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ctHeader = "Content-Type"

	JsonContentType     = "application/json"
	JsonContentTypeUTF8 = JsonContentType + "; charset=UTF-8"
	TextHtmlContentType = "text/html"

	// If there statusCode written to the context,
	// this default will be written to the response.
	defaultStatusCode int = http.StatusOK

	// By default there is a maximum of 10MB size of formBody.
	defaultMaxFormBodySize uint64 = 10 << 20

	bindedValueKey bindValueKey = "bindedValues"

	routeParamsKey contextKey = "__routeParams__"
)

type (
	pathParams    map[string]string
	contextIdChan <-chan uint64
	anyValue      interface{}

	bindValueKey string
	contextKey   string

	contextMap map[contextKey]anyValue
)

type responseWriter struct {
	statusCode int
	header     http.Header
	b          []byte

	w http.ResponseWriter
}

type context struct {
	ctx     ctxpkg.Context
	writer  *responseWriter
	request *http.Request

	body []byte

	contextId     uint64
	contextIdChan contextIdChan
	startTime     time.Time
	maxBodySize   uint64
}

type Context interface {
	// ---- Non public package related
	reset(http.ResponseWriter, *http.Request)
	empty()

	// ---- Request
	GetRequest() *http.Request
	GetRequestMethod() string
	GetUrl() string
	GetCleanedUrl() string
	GetQueryParams() url.Values
	GetQueryParam(string) string
	BindValue(contextKey, any)
	GetBindedValue(contextKey) any
	GetRequestHeader(string) string
	GetContentType() string
	GetParam(string) string
	GetParams() pathParams
	GetRequestHeaders() http.Header
	GetBody() []byte

	// TODO:
	// – FormData
	// – Parse()
	// – GetFormValue()
	// – GetFormFile()

	// ---- Response
	SendJson(anyValue)
	SendNotFound()
	SendInternalServerError()
	SendMethodNotAllowed()
	SendOk()
	SendUnauthorized()
	SendRaw([]byte, int, http.Header)
	Pipe(*http.Response)
	SetStatusCode(int)
	WriteResponse(b []byte)
	AppendHttpHeader(header http.Header)
	WriteToResponseNow()
}

var _ Context = (*context)(nil)

// newContext creates and returns a new context.
func newContext(ciChan contextIdChan, defaultStatusCode int, maxBodySize uint64) *context {
	return &context{
		contextIdChan: ciChan,
		writer:        newResponseWriter(defaultStatusCode),
		maxBodySize:   maxBodySize,
	}
}

func newResponseWriter(statusCode int) *responseWriter {
	return &responseWriter{
		statusCode: statusCode,
		header:     http.Header{},
	}
}

// reset resets the context entity to default state.
func (ctx *context) reset(w http.ResponseWriter, r *http.Request) {
	ctx.ctx = ctxpkg.Background()
	ctx.writer.w = w
	ctx.request = r
	ctx.startTime = time.Now()

	// We set the next id from the channel.
	if ctx.contextIdChan != nil {
		ctx.contextId = <-ctx.contextIdChan
	}
}

// empty makes the http.Request and http.ResponseWrite <nil>.
// Should be called before putting the Context back to the pool.
func (c *context) empty() {
	c.discard()

	c.request = nil
	c.writer.empty()
}

// GetRequest returns the attached http.Request pointer.
func (ctx *context) GetRequest() *http.Request {
	return ctx.request
}

// GetRequestMethod returns the method of incoming request.
func (ctx *context) GetRequestMethod() string {
	if ctx == nil {
		return ""
	}
	if ctx.request == nil {
		return ""
	}
	return ctx.request.Method
}

// BindValue binds a given value – with any – to the ongoing request with certain key.
func (ctx *context) BindValue(key contextKey, value any) {
	bindedValues, ok := ctx.ctx.Value(bindedValueKey).(contextMap)
	if bindedValues == nil || !ok {
		bindedValues = make(contextMap, 0)
		ctx.ctx = ctxpkg.WithValue(ctx.ctx, bindedValueKey, bindedValues)
	}
	bindedValues[key] = value
}

// GetBindedValue returns the binded from the request.
func (ctx *context) GetBindedValue(key contextKey) any {
	bindedValues := ctx.ctx.Value(bindedValueKey).(contextMap)
	if bindedValues == nil {
		return nil
	}
	return bindedValues[key]
}

// GetlUrl returns the full URL with all queryParams included.
func (ctx *context) GetUrl() string {
	if ctx.request == nil {
		return ""
	}
	return ctx.request.RequestURI
}

// GetCleanedUrl returns the url
// without query params, it there is any.
func (ctx *context) GetCleanedUrl() string {
	return removeQueryPart(ctx.GetUrl())
}

// GetQueryParams returns the query params of the url.
func (ctx *context) GetQueryParams() url.Values {
	return ctx.request.URL.Query()
}

// GetQueryParam returns the queryParam identified by the given key.
func (ctx *context) GetQueryParam(key string) string {
	query := ctx.GetQueryParams()

	return query.Get(key)
}

// GetBody returns the body read from the incoming request.
func (ctx *context) GetBody() []byte {
	return ctx.body
}

// GetRequestHeaders returns all the headers from the request.
func (ctx *context) GetRequestHeaders() http.Header {
	return ctx.request.Header
}

// GetRequestHeader return one specific headers value, with given key.
func (ctx *context) GetRequestHeader(key string) string {
	header := ctx.GetRequestHeaders()

	return header.Get(key)
}

// GetContentType returns te content-type of the original request.
func (ctx *context) GetContentType() string {
	return ctx.GetRequestHeader(ctHeader)
}

// GetParam returns the value of the param identified by the given key.
func (ctx *context) GetParam(key string) string {
	return ctx.GetParams()[key]
}

// GetParams returns all the path params associated with thre context.
func (ctx *context) GetParams() pathParams {
	params, ok := ctx.GetBindedValue(routeParamsKey).(pathParams)
	if !ok {
		return map[string]string{}
	}
	return params
}

// SendRaw writes the given slice of bytes, statusCode and header to the response.
func (ctx *context) SendRaw(b []byte, statusCode int, header http.Header) {
	ctx.WriteResponse(b)
	ctx.SetStatusCode(statusCode)
	ctx.AppendHttpHeader(header)
}

// WriteResponse writes the given slice of bytes to the response.
func (ctx *context) WriteResponse(b []byte) {
	ctx.writer.write(b)
}

// SetStatusCode sets the statusCode for the response.
func (ctx *context) SetStatusCode(statusCode int) {
	ctx.writer.setStatus(statusCode)
}

// SendsJson send a JSON response to client.
func (ctx *context) SendJson(data anyValue) {
	b, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("marshal err: %v\n", err)

		return
	}

	ctx.SendRaw(b, http.StatusOK, createContentTypeHeader(JsonContentType))
}

func createContentTypeHeader(ct string) http.Header {
	header := http.Header{}

	header.Add(ctHeader, ct)

	return header
}

// SendOk send a s basic HTTP 200 response.
func (ctx *context) SendOk() {
	ctx.SendRaw(nil, http.StatusOK, http.Header{})
}

// SendNotFound sends a HTTP 404 error.
func (ctx *context) SendNotFound() {
	ctx.SendHttpError(http.StatusNotFound)
}

// SendMethodNotAllowed sends a HTTP 405 error.
func (ctx *context) SendMethodNotAllowed() {
	ctx.SendHttpError(http.StatusMethodNotAllowed)
}

// SendUnauthorized send a HTTP 401 error.
func (ctx *context) SendUnauthorized() {
	ctx.SendHttpError(http.StatusUnauthorized)
}

// SendInternalServerError send a HTTP 500 error.
func (ctx *context) SendInternalServerError() {
	ctx.SendHttpError(http.StatusInternalServerError)
}

// SendUnavailable send a HTTP 503 error.
func (ctx *context) SendUnavailable() {
	ctx.SendHttpError(http.StatusServiceUnavailable)
}

// SendHttpError send HTTP error with the given code.
// It also write the statusText inside the body, based on the code.
func (ctx *context) SendHttpError(statusCode int) {
	ctx.SetStatusCode(statusCode)
}

// Pipe writes the given repsonse's body, statusCode and headers to the Context's response.
func (ctx *context) Pipe(res *http.Response) {
	// We could use TeeReader if we want to know
	// what are we writing to the request.
	// r := io.TeeReader(res.Body, ctx.writer)
	ctx.writer.copy(res.Body)
	ctx.AppendHttpHeader(res.Header)
	ctx.SetStatusCode(res.StatusCode)
}

// AppendHttpHeader appends all the key-value pairs from the given
// http.Header to the responses header.
func (ctx *context) AppendHttpHeader(header http.Header) {
	for k, v := range header {
		ctx.writer.addHeader(k, strings.Join(v, "; "))
	}
}

// WriteToResponseNow writes the actual response of context to the underlying connection.
func (ctx *context) WriteToResponseNow() {
	ctx.writer.writeToResponse()
}

func (ctx *context) discard() {
	m := ctx.GetRequestMethod()

	if m != http.MethodPost && m != http.MethodPut {
		return
	}

	reader := ctx.GetRequest().Body

	// Just in case we always read and discard the request body
	if _, err := io.Copy(io.Discard, reader); err != nil {
		// If the error is the reading after close, we simply ignore it.
		if !errors.Is(err, http.ErrBodyReadAfterClose) {
			fmt.Println(err)
		}
		return
	}
	reader.Close()
}

func (rw *responseWriter) empty() {
	rw.b = rw.b[:0]
	rw.header = http.Header{}
	rw.statusCode = 0
	rw.w = nil
}

func (rw *responseWriter) write(b []byte) {
	rw.b = b
}

func (rw *responseWriter) setStatus(statusCode int) {
	rw.statusCode = statusCode
}

func (rw *responseWriter) addHeader(key, value string) {
	rw.header.Add(key, value)
}

func (rw *responseWriter) copy(r io.Reader) {
	buff := &bytes.Buffer{}

	if _, err := io.Copy(buff, r); err != nil {
		fmt.Println(err)
		return
	}

	rw.b = buff.Bytes()
}

func (rw *responseWriter) writeToResponse() {
	if len(rw.b) == 0 && rw.statusCode >= http.StatusMultipleChoices {
		http.Error(rw.w, http.StatusText(rw.statusCode), rw.statusCode)
		return
	}

	for k, v := range rw.header {
		value := strings.Join(v, ";")
		rw.w.Header().Add(k, value)
	}

	finalStatusCode := func() int {
		if rw.statusCode == 0 {
			// TODO: change it to logger
			fmt.Printf("attempted to write '0' status\n")
			// Just in case there is a fallback to it.
			return defaultStatusCode
		}
		return rw.statusCode
	}()

	rw.w.WriteHeader(finalStatusCode)
	rw.w.Write(rw.b)
}

type contextLog struct {
	method      string
	url         string
	code        int
	elapsedTime int64
	contextId   uint64
}

func (ctx *context) getLog() contextLog {
	elapsedTime := time.Since(ctx.startTime)

	return contextLog{
		method:      ctx.GetRequestMethod(),
		url:         ctx.GetUrl(),
		code:        ctx.writer.statusCode,
		elapsedTime: elapsedTime.Milliseconds(),
		contextId:   ctx.contextId,
	}
}
