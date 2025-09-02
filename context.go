package gorouter

import (
	"bytes"
	ctxpkg "context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	contentTypeHeaderKey string = "Content-Type"

	MultiPartFormContentType string = "multipart/form-data"

	// If there statusCode written to the context,
	// this default will be written to the response.
	defaultStatusCode int = http.StatusOK

	// By default there is a maximum of 10MB size of formBody.
	defaultMaxFormBodySize int64 = 10 << 20

	bindedValueKey bindValueKey = "bindedValues"

	routeParamsKey   ContextKey = "__routeParams__"
	queryParamsKey   ContextKey = "__queryParams__"
	reqisteredUrlKey ContextKey = "__registeredUrl__"
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
	writer  *responseWriter
	request *http.Request

	contextId     uint64
	contextIdChan contextIdChan
	startTime     time.Time
	maxBodySize   int64

	isFormParsed bool

	index uint8
}

type ContextInfo struct {
	Id           uint64
	WrittenBytes int64
	StartTime    time.Time
	Url          string
	StatusCode   int
	Method       string
}

type Context interface {
	// ---- Methods about the Context itself.
	Reset(http.ResponseWriter, *http.Request)
	Empty()
	GetContextId() uint64
	GetCurrentIndex() uint8
	GetStartTime() time.Time
	Next()
	GetInfo() ContextInfo

	// ---- Request
	GetRequest() *http.Request
	GetRequestMethod() string
	GetUrl() string
	GetCleanedUrl() string
	GetRegisteredUrl() string
	GetQueryParams() url.Values
	GetQueryParam(key string) string
	BindValue(key ContextKey, value any)
	GetBindedValue(key ContextKey) any
	GetRequestHeader(key string) string
	GetContentType() string
	GetRequestHeaders() http.Header
	GetBody() io.ReadCloser
	ParseForm() error
	GetFormFile(string) (File, error)
	GetFormValue(string) (string, error)
	GetParam(key string) string
	GetIntParam(key string) (int, error)
	GetInt8Param(key string) (int8, error)
	GetInt16Param(key string) (int16, error)
	GetInt32Param(key string) (int32, error)
	GetInt64Param(key string) (int64, error)
	GetFloat32Param(key string) (float32, error)
	GetFloat64Param(key string) (float64, error)
	GetParams() pathParams

	// ---- Response
	Pipe(res *http.Response)
	Status(statusCode int)
	StatusText(statusCode int)
	AppendHttpHeader(key string, value string)
	Flush()
	Copy(io.Reader)
	Render(statusCode int, r Response)
	SendJson(statusCode int, data any)
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
	WriteTo(w io.Writer) (int64, error)
	SaveTo(string) error
}

type ContextConfig struct {
	ContextIdChannel          contextIdChan
	DefaultResponseStatusCode int
	MaxIncomingBodySize       int64
}

// NewContext creates and returns a new context.
func NewContext(conf ContextConfig) *context {
	return &context{
		contextIdChan: conf.ContextIdChannel,
		writer:        newResponseWriter(conf.DefaultResponseStatusCode),
		maxBodySize:   conf.MaxIncomingBodySize,
		index:         1,
	}
}

func newResponseWriter(statusCode int) *responseWriter {
	return &responseWriter{
		defaultStatusCode: statusCode,
		buff:              &bytes.Buffer{},
	}
}

// Reset Resets the context entity to default state.
func (ctx *context) Reset(w http.ResponseWriter, r *http.Request) {
	ctx.ctx = ctxpkg.Background()
	ctx.writer.w = w
	ctx.request = r
	ctx.startTime = time.Now()

	// We set the next id from the channel.
	if ctx.contextIdChan != nil {
		ctx.contextId = <-ctx.contextIdChan
	}
}

// Empty makes the http.Request and http.ResponseWrite <nil>.
// Should be called before putting the Context back to the pool.
func (ctx *context) Empty() {
	ctx.discard()
	ctx.request = nil
	ctx.writer.Empty()
	ctx.index = 1
}

// GetContextId returns the id of the context entity.
func (ctx *context) GetContextId() uint64 {
	return ctx.contextId
}

// GetCurrentIndex returns the current index,
// which translates directly to how many times
// the Next() function was called to that point.
func (ctx *context) GetCurrentIndex() uint8 {
	return ctx.index
}

// GetStartTime returns the time when the execution of the current
// context has started.
func (ctx *context) GetStartTime() time.Time {
	return ctx.startTime
}

// GetInfo returns minimal information about the given context.
func (ctx *context) GetInfo() ContextInfo {
	return ContextInfo{
		Id:           ctx.contextId,
		WrittenBytes: int64(ctx.writer.writtenBytes),
		StartTime:    ctx.startTime,
		Url:          ctx.GetUrl(),
		StatusCode:   ctx.writer.statusCode,
		Method:       ctx.request.Method,
	}
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

// GetRegisteredUrl returns the exact url,
// with which the current endpoint was registerd.
func (ctx *context) GetRegisteredUrl() string {
	if url, ok := ctx.GetBindedValue(reqisteredUrlKey).(string); ok {
		return url
	}
	return ""
}

// GetQueryParams returns the query params of the url.
func (ctx *context) GetQueryParams() url.Values {
	query, ok := ctx.GetBindedValue(queryParamsKey).(url.Values)
	if !ok {
		query = ctx.request.URL.Query()

		ctx.BindValue(queryParamsKey, query)
	}

	return query
}

// GetQueryParam returns the queryParam identified by the given key.
func (ctx *context) GetQueryParam(key string) string {
	query := ctx.GetQueryParams()

	return query.Get(key)
}

// GetBody returns the body of the incoming request.
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
	return ctx.GetRequestHeader(contentTypeHeaderKey)
}

// GetParam returns the value of the param identified by the given key.
func (ctx *context) GetParam(key string) string {
	return ctx.GetParams()[key]
}

// GetIntParam returns the parsed integer value of the param identified by the given key.
func (ctx *context) GetIntParam(key string) (int, error) {
	v := ctx.GetParams()[key]
	return strconv.Atoi(v)
}

// GetInt8Param returns the parsed int8 value of the param identified by the given key.
func (ctx *context) GetInt8Param(key string) (int8, error) {
	i, err := ctx.GetIntParam(key)
	if err != nil {
		return 0, nil
	}
	return int8(i), nil
}

// GetInt16Param returns the parsed int16 value of the param identified by the given key.
func (ctx *context) GetInt16Param(key string) (int16, error) {
	i, err := ctx.GetIntParam(key)
	if err != nil {
		return 0, nil
	}
	return int16(i), nil
}

// GetInt32Param returns the parsed int32 value of the param identified by the given key.
func (ctx *context) GetInt32Param(key string) (int32, error) {
	i, err := ctx.GetIntParam(key)
	if err != nil {
		return 0, nil
	}
	return int32(i), nil
}

// GetInt64Param returns the parsed int64 value of the param identified by the given key.
func (ctx *context) GetInt64Param(key string) (int64, error) {
	i, err := ctx.GetIntParam(key)
	if err != nil {
		return 0, nil
	}
	return int64(i), nil
}

// GetFloat32Param returns the parsed float32 value of the param identified by the given key.
func (ctx *context) GetFloat32Param(key string) (float32, error) {
	f, err := ctx.GetFloat64Param(key)
	if err != nil {
		return 0.0, err
	}
	return float32(f), nil
}

// GetFloat64Param returns the parsed float64 value of the param identified by the given key.
func (ctx *context) GetFloat64Param(key string) (float64, error) {
	return strconv.ParseFloat(ctx.GetParam(key), 64)
}

// GetParams returns all the path params associated with thre context.
func (ctx *context) GetParams() pathParams {
	bindedValue := ctx.GetBindedValue(routeParamsKey)
	if bindedValue == nil {
		return map[string]string{}
	}
	params, ok := bindedValue.(pathParams)
	if !ok {
		fmt.Println("not ok")
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

// WriteResponse writes the given slice of bytes to the response.
func (ctx *context) WriteResponse(b []byte) {
	ctx.writer.write(b)
}

// Status sets the status code.
func (ctx *context) Status(statusCode int) {
	ctx.writer.setStatus(statusCode)
}

// StatusText sets the status code with the associated status text.
func (ctx *context) StatusText(statusCode int) {
	t := http.StatusText(statusCode)

	ctx.Render(statusCode, &DefaultResponse{Data: []byte(t)})
}

// Render renders the given response with the provided status code.
func (ctx *context) Render(statusCode int, r Response) {
	ctx.Status(statusCode)
	ctx.writer.render(r)
}

// SendJson writes JSON response to the request.
func (ctx *context) SendJson(statusCode int, data any) {
	ctx.Render(statusCode, &JsonResponse{Data: data})
}

// Pipe writes the given repsonse's body, statusCode and headers to the Context's response.
func (ctx *context) Pipe(res *http.Response) {
	// We could use TeeReader if we want to know
	// what are we writing to the request.
	// r := io.TeeReader(res.Body, ctx.writer)
	ctx.writer.copy(res.Body)
	for k, v := range res.Header {
		for _, e := range v {
			ctx.AppendHttpHeader(k, e)
		}
	}
	ctx.Status(res.StatusCode)
}

// AppendHttpHeader appends all the key-value pairs from the given
// http.Header to the responses header.
func (ctx *context) AppendHttpHeader(key string, value string) {
	ctx.writer.addHeader(key, value)
}

// WriteToResponseNow writes the actual response of context to the underlying connection.
func (ctx *context) Flush() {
	ctx.writer.flush()
}

// Copy copies the content of the given reader to the response writer.
func (ctx *context) Copy(r io.Reader) {
	ctx.writer.copy(r)
}

// Next increments the index of the given context,
// thus signalling to the engine, that the next entity
// should be called in the handler chain.
func (ctx *context) Next() {
	ctx.index += 1
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
