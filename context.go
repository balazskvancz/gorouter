package gorouter

import (
	ctxpkg "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	ctHeader string = "Content-Type"

	JsonContentType          string = "application/json"
	JsonContentTypeUTF8      string = JsonContentType + "; charset=UTF-8"
	TextHtmlContentType      string = "text/html"
	MultiPartFormContentType string = "multipart/form-data"

	// If there statusCode written to the context,
	// this default will be written to the response.
	defaultStatusCode int = http.StatusOK

	// By default there is a maximum of 10MB size of formBody.
	defaultMaxFormBodySize int64 = 10 << 20

	bindedValueKey bindValueKey = "bindedValues"

	routeParamsKey  ContextKey = "__routeParams__"
	incomingBodyKey ContextKey = "__incomingBody__"
)

var (
	ErrCtNotMultipart = errors.New("the content-type is not multipart/form-data")
)

type (
	pathParams    map[string]string
	contextIdChan <-chan uint64
	anyValue      interface{}

	bindValueKey string
	ContextKey   string

	contextMap map[ContextKey]anyValue
)

type context struct {
	ctx     ctxpkg.Context
	conn    net.Conn
	writer  *responseWriter
	request *http.Request

	contextId     uint64
	contextIdChan contextIdChan
	startTime     time.Time
	maxBodySize   int64

	isFormParsed bool

	logger Logger
}

type Context interface {
	Logger

	// ---- Methods about the Context itself.
	Reset(http.ResponseWriter, *http.Request)
	Empty()
	GetContextId() uint64
	Close()

	// ---- Request
	GetRequest() *http.Request
	GetRequestMethod() string
	GetUrl() string
	GetCleanedUrl() string
	GetQueryParams() url.Values
	GetQueryParam(key string) string
	BindValue(ContextKey, any)
	GetBindedValue(ContextKey) any
	GetRequestHeader(header string) string
	GetContentType() string
	GetParam(param string) string
	GetParams() pathParams
	GetRequestHeaders() http.Header
	GetBody() io.ReadCloser
	ParseForm() error
	GetFormFile(key string) (File, error)
	GetFormValue(key string) (string, error)

	// ---- Response
	SendJson(anyValue, ...int)
	SendNotFound()
	SendInternalServerError()
	SendMethodNotAllowed()
	SendOk()
	SendUnauthorized()
	SendRaw(b []byte, code int, header http.Header)
	Pipe(res *http.Response)
	AppendHttpHeader(header http.Header)
	Flush() error
	Copy(r io.Reader)

	GetLog() *contextLog
}

var _ Context = (*context)(nil)

type formFile struct {
	file   multipart.File
	header *multipart.FileHeader
}

type File interface {
	GetName() string
	GetSize() int64
	Close() error
	WriteTo(io.Writer) (int64, error)
	SaveTo(string) error
}

type ContextConfig struct {
	ContextIdChannel          contextIdChan
	DefaultResponseStatusCode int
	MaxIncomingBodySize       int64
	Logger                    Logger
}

// NewContext creates and returns a new context.
func NewContext(conf ContextConfig) *context {
	return &context{
		contextIdChan: conf.ContextIdChannel,
		writer:        newResponseWriter(conf.DefaultResponseStatusCode),
		maxBodySize:   conf.MaxIncomingBodySize,
		logger:        conf.Logger,
	}
}

// Reset Resets the context entity to default state.
func (ctx *context) Reset(w http.ResponseWriter, r *http.Request) {
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		// TODO:
		fmt.Println(err)
	}

	ctx.ctx = ctxpkg.Background()
	ctx.writer.w = rw
	ctx.writer.setVersion(r.Proto)
	ctx.request = r
	ctx.startTime = time.Now()
	ctx.conn = conn

	// We set the next id from the channel.
	if ctx.contextIdChan != nil {
		ctx.contextId = <-ctx.contextIdChan
	}
}

// Empty makes the http.Request and http.ResponseWrite <nil>.
// Should be called before putting the Context back to the pool.
func (c *context) Empty() {
	c.discard()

	c.request = nil
	c.conn = nil
	c.writer.Empty()
}

func (c *context) Close() {
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			fmt.Println(err)
		}
	}
}

// GetContextId returns the id of the context entity.
func (c *context) GetContextId() uint64 {
	return c.contextId
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
func (ctx *context) BindValue(key ContextKey, value any) {
	bindedValues, ok := ctx.ctx.Value(bindedValueKey).(contextMap)
	if bindedValues == nil || !ok {
		bindedValues = make(contextMap, 0)
		ctx.ctx = ctxpkg.WithValue(ctx.ctx, bindedValueKey, bindedValues)
	}
	bindedValues[key] = value
}

// GetBindedValue returns the binded from the request.
func (ctx *context) GetBindedValue(key ContextKey) any {
	val := ctx.ctx.Value(bindedValueKey)
	if val == nil {
		return nil
	}
	bindedValues := val.(contextMap)
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

// GetBody returns the body of the incoming request.
// Closing the body is the callers responsibility.
func (ctx *context) GetBody() io.ReadCloser {
	return ctx.request.Body
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
	bindedValue := ctx.GetBindedValue(routeParamsKey)
	if bindedValue == nil {
		return map[string]string{}
	}
	params, ok := bindedValue.(pathParams)
	if !ok {
		return map[string]string{}
	}
	return params
}

// ParseForm tries to parse the incoming request as a formdata. Returns error
// if the content-type is not valid, or the native parse returns error.
func (ctx *context) ParseForm() error {
	if ctx.isFormParsed {
		return nil
	}
	if !strings.Contains(ctx.GetContentType(), MultiPartFormContentType) {
		return ErrCtNotMultipart
	}
	ctx.isFormParsed = true
	return ctx.request.ParseMultipartForm(ctx.maxBodySize)
}

// GetFormValue returns the value in the form associated with
// the given key. It calls ParseForm, if has to.
func (ctx *context) GetFormValue(key string) (string, error) {
	if !ctx.isFormParsed {
		if err := ctx.ParseForm(); err != nil {
			return "", err
		}
	}
	return ctx.request.FormValue(key), nil
}

// GetFormFile returns the File and and error associated with
// the given key. It calls ParseForm, if has to.
func (ctx *context) GetFormFile(key string) (File, error) {
	if !ctx.isFormParsed {
		if err := ctx.ParseForm(); err != nil {
			return nil, err
		}
	}
	file, header, err := ctx.request.FormFile(key)
	if err != nil {
		return nil, err
	}
	return &formFile{
		file:   file,
		header: header,
	}, nil
}

// SendRaw writes the given slice of bytes, statusCode and header to the response.
func (ctx *context) SendRaw(b []byte, statusCode int, header http.Header) {
	ctx.writer.write(b)
	ctx.setStatusCode(statusCode)
	ctx.AppendHttpHeader(header)
}

// SendsJson send a JSON response to client.
func (ctx *context) SendJson(data anyValue, code ...int) {
	if err := json.NewEncoder(ctx.writer.buff).Encode(data); err != nil {
		ctx.Error("[SendJson error]: %v\n", err)

		return
	}

	statusCode := http.StatusOK
	if len(code) > 0 {
		statusCode = code[0]
	}

	ctx.setStatusCode(statusCode)

	ctx.AppendHttpHeader(http.Header{
		"Content-Type": []string{JsonContentTypeUTF8},
	})
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
	ctx.setStatusCode(statusCode)
}

// Pipe writes the given repsonse's body, statusCode and headers to the Context's response.
func (ctx *context) Pipe(res *http.Response) {
	// We could use TeeReader if we want to know
	// what are we writing to the request.
	// r := io.TeeReader(res.Body, ctx.writer)
	ctx.writer.copy(res.Body)
	ctx.AppendHttpHeader(res.Header)
	ctx.setStatusCode(res.StatusCode)
}

// AppendHttpHeader appends all the key-value pairs from the given
// http.Header to the responses header.
func (ctx *context) AppendHttpHeader(header http.Header) {
	for k, v := range header {
		ctx.writer.addHeader(k, strings.Join(v, ", "))
	}
}

// Flush writes the actual response of context to the underlying connection.
func (ctx *context) Flush() error {
	return ctx.writer.flush()
}

// Copy copies the content of the given reader to the response writer.
func (ctx *context) Copy(r io.Reader) {
	ctx.writer.copy(r)
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

// FormFile.

// GetName returns the name of the attached file.
func (ff *formFile) GetName() string {
	return ff.header.Filename
}

// GetSize returns the size of the attached file.
func (ff *formFile) GetSize() int64 {
	return ff.header.Size
}

// Close closes the file.
func (ff *formFile) Close() error {
	return ff.file.Close()
}

// WriteTo writes the content of the file to a certain Writer.
func (ff *formFile) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, ff.file)
}

// SaveTo saves a certain file to the given location.
func (ff *formFile) SaveTo(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	_, err = ff.WriteTo(file)
	return err
}

// Info handles info type logging.
func (ctx *context) Info(format string, v ...any) {
	ctx.logger.Info(fmt.Sprintf("[%d] %s", ctx.contextId, format), v...)
}

// Warning handles warning type logging.
func (ctx *context) Warning(format string, v ...any) {
	ctx.logger.Warning(fmt.Sprintf("[%d] %s", ctx.contextId, format), v...)
}

// Error handles error type logging.
func (ctx *context) Error(format string, v ...any) {
	ctx.logger.Error(fmt.Sprintf("[%d] %s", ctx.contextId, format), v...)
}

type contextLog struct {
	method      string
	url         string
	code        int
	elapsedTime int64
	contextId   uint64
}

func (ctx *context) GetLog() *contextLog {
	elapsedTime := time.Since(ctx.startTime)

	return &contextLog{
		method:      ctx.GetRequestMethod(),
		url:         ctx.GetUrl(),
		code:        ctx.writer.statusCode,
		elapsedTime: elapsedTime.Milliseconds(),
		contextId:   ctx.contextId,
	}
}

func (cl *contextLog) Serialize() string {
	return fmt.Sprintf("[%s]\t%s\t%d\t%dms", cl.method, cl.url, cl.code, cl.elapsedTime)
}

func (ctx *context) setStatusCode(statusCode int) {
	if !isValidStatusCode(statusCode) {
		ctx.Warning(
			"[SetStatusCode]: given %d is invalid HTTP status code; instead the the fallback (%d) is used\n",
			statusCode, ctx.writer.defaultStatusCode,
		)

		return
	}

	ctx.writer.setStatus(statusCode)
}
