package gorouter

import (
	ctxpkg "context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type Router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)

	Serve(Context)
	ListenWithContext(ctxpkg.Context)
	Listen()
	RegisterMiddlewares(middlewares ...Middleware)
	RegisterPostMiddlewares(middlewares ...Middleware)

	// All the available methods to register:
	Get(string, HandlerFunc) Route
	Post(string, HandlerFunc) Route
	Put(string, HandlerFunc) Route
	Delete(string, HandlerFunc) Route
	Head(string, HandlerFunc) Route
}

type (
	HandlerFunc      func(Context)
	MiddlewareFunc   func(Context, HandlerFunc)
	PanicHandlerFunc func(Context, interface{})

	routerOptionFunc func(*router)

	bodyReaderFn func(*http.Request) []byte
)

const (
	version string = "v1.3.0"

	defaultAddress    int    = 8000
	defaultServerName string = "goRouter"
)

var (
	// The automatic reading of the incoming postData – if it is enabled –
	// is only done, if the method of the incoming request is one these methods.
	couldReadBody []string = []string{http.MethodPost, http.MethodPut}

	// Also connects to body reading. If the content type of
	// the incoming request is one of the listed types,
	// then the automatic reading is not perfomed.
	// Note that, in case of this inspection we look for substring match
	// not for exact match.
	// For now, only the multipart/form-data content type is not read
	// by default, maybe should change it automatic parseing instead of reading.
	exceptionContentTypes []string = []string{MultiPartFormContentType}
)

type routerInfo struct {
	// The name of the specific router instance.
	serverName string

	// The address where the router will be listening.
	address int

	// The maximum size of the body in case in multipart/form-data content.
	maxFormSize int64

	// The default statusCode of the response.
	// If no response is written during the execution,
	// then this code will be written to the response.
	defaultResponseStatusCode int

	//
	areMiddlewaresEnabled bool
}

type router struct {
	routerInfo

	// The base running context of the router, use it for cancellation.
	ctx ctxpkg.Context

	// Tree for all the registered endpoints.
	endpointTree *node

	// Instead of creating a new Context for each incoming request
	// we use this pool to acquire an already initiated entity,
	// and after the initiation, we return it.
	//
	// NOTE: everytime we put one entity back to the pool, the caller
	// must Empty all the attached pointers, otherwise the GC won't be
	// able to free space of memory.
	contextPool sync.Pool

	// The registry for all the globally registered middlwares.
	// We store two different types of middlewares.
	// There is one for before all execution and one
	// for after all execution order.
	// middlewares []Middleware
	middlewares middlewareRegistry

	// Custom handler for HTTP 404. Everytime a specific
	// route is not found or a service returned 404 it gets called.
	// By default, there a default notFoundHandler, which sends 404 in header.
	notFoundHandler HandlerFunc

	// Custom handler for HTTP OPTIONS.
	optionsHandler HandlerFunc

	// Custom handler function for panics.
	panicHandler PanicHandlerFunc

	// A handler when the method tree is empty.
	emptyTreeHandler HandlerFunc

	// A custom function to read the body of the
	// incoming request in advance.
	bodyReader bodyReaderFn

	logger Logger
}

// WithAddress allows to configure address of the router
// where it will be listening.
func WithAddress(address int) routerOptionFunc {
	return func(r *router) {
		if address > 0 {
			r.routerInfo.address = address
		}
	}
}

// WithMaxBodySize allows to configure maximum
// size incoming, decodable formdata.
func WithMaxBodySize(size int64) routerOptionFunc {
	return func(r *router) {
		if size > 0 {
			r.routerInfo.maxFormSize = size
		}
	}
}

// WithDefaultStatusCode allows to configure the default
// statusCode of the response without specifying it explicitly.
func WithDefaultStatusCode(statusCode int) routerOptionFunc {
	return func(r *router) {
		if statusCode > 0 {
			r.routerInfo.defaultResponseStatusCode = statusCode
		}
	}
}

// WithContext allows to configure basecontext of the router
// which will be passed to each and every handler.
func WithContext(ctx ctxpkg.Context) routerOptionFunc {
	return func(r *router) {
		if ctx != nil {
			r.ctx = ctx
		}
	}
}

// WithNotFoundHandler allows to configure 404 handler of the router.
func WithNotFoundHandler(h HandlerFunc) routerOptionFunc {
	return func(r *router) {
		r.notFoundHandler = h
	}
}

// WithOptionsHandler allows to configure OPTIONS method handler of the router.
func WithOptionsHandler(h HandlerFunc) routerOptionFunc {
	return func(r *router) {
		r.optionsHandler = h
	}
}

// WithPanicHandler allows to configure a recover function
// which is called if a panic happens somewhere.
func WithPanicHandler(h PanicHandlerFunc) routerOptionFunc {
	return func(r *router) {
		r.panicHandler = h
	}
}

// WithBodyReader allows to configure a default body reader function
// or disable it (by passing in <nil>).
func WithBodyReader(reader bodyReaderFn) routerOptionFunc {
	return func(r *router) {
		r.bodyReader = reader
	}
}

// WithServerName allows to configure the server name of the instance.
func WithServerName(name string) routerOptionFunc {
	return func(r *router) {
		r.routerInfo.serverName = name
	}
}

// WithMiddlewaresEnabled allows to configure the state of the middlewares.
func WithMiddlewaresEnabled(areEnabled bool) routerOptionFunc {
	return func(r *router) {
		r.routerInfo.areMiddlewaresEnabled = areEnabled
	}
}

// WithEmptyTreeHandler allows to configure the handler in case of an empty method tree event.
func WithEmptyTreeHandler(handler HandlerFunc) routerOptionFunc {
	return func(r *router) {
		r.emptyTreeHandler = handler
	}
}

// New returns a new Router instance decorated
// by the given optionFuncs.
func New(opts ...routerOptionFunc) Router {
	ctxIdChannel := getContextIdChan()

	r := &router{
		routerInfo: routerInfo{
			serverName:                defaultServerName,
			address:                   defaultAddress,
			defaultResponseStatusCode: defaultStatusCode,
			maxFormSize:               defaultMaxFormBodySize,
			areMiddlewaresEnabled:     true, // By default every middleware are enabled.
		},

		// By deafult we simply use the Background context.
		ctx: ctxpkg.Background(),

		middlewares:  make(middlewareRegistry, 0),
		endpointTree: newNode(),

		notFoundHandler:  defaultNotFoundHandler,
		emptyTreeHandler: defaultEmptyTreeHandler,
		optionsHandler:   nil,
		panicHandler:     nil,

		bodyReader: defaultBodyReader,
	}

	for _, o := range opts {
		o(r)
	}

	logger := newLogger(r.routerInfo.serverName)

	r.logger = logger

	r.contextPool = sync.Pool{
		New: func() any {
			return NewContext(ContextConfig{
				ContextIdChannel:          ctxIdChannel,
				DefaultResponseStatusCode: r.routerInfo.defaultResponseStatusCode,
				MaxIncomingBodySize:       r.maxFormSize,
				Logger:                    logger,
			})
		},
	}

	// If it was not disabled during by the opts,
	// then we append the middleware to preRunners.
	if r.bodyReader != nil {
		r.RegisterMiddlewares(r.getBodyReaderMiddleware())
	}

	// Registering the logger and writer middleware to the postrunners.
	r.RegisterPostMiddlewares(
		getWriterPostMiddleware(), // First we write the response to the connection,
		getLoggerMiddleware(),     // then log write the log to stdout.
	)

	return r
}

// Listen starts the HTTP listening on the specified address.
func (r *router) Listen() {
	r.ListenWithContext(r.ctx)
}

// ListenWithContext starts the HTTP listening with cancellation
// bounded to the given context.
func (r *router) ListenWithContext(ctx ctxpkg.Context) {
	addr := fmt.Sprintf(":%d", r.address)
	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			r.logger.Error(err.Error())
		}
	}()

	signalChannel := make(chan os.Signal, 1)

	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	r.logger.Info("started listening at %s", addr)

	var shutdown = func() {
		r.logger.Info("the router is shutting down...")
		if err := server.Shutdown(ctxpkg.Background()); err != nil {
			r.logger.Error(err.Error())
		}
	}

	select {
	case <-signalChannel:
		shutdown()
	case <-ctx.Done():
		shutdown()
	}
	r.logger.Info("the router is shutted down...")
}

// Get registers creates and returns new route with HTTP GET method.
func (r *router) Get(url string, handler HandlerFunc) Route {
	return r.addRoute(http.MethodGet, url, handler)
}

// Post registers creates and returns new route with HTTP POST method.
func (r *router) Post(url string, handler HandlerFunc) Route {
	return r.addRoute(http.MethodPost, url, handler)
}

// Put registers creates and returns new route with HTTP PUT method.
func (r *router) Put(url string, handler HandlerFunc) Route {
	return r.addRoute(http.MethodPut, url, handler)
}

// Delete registers creates and returns new route with HTTP DELETE method.
func (r *router) Delete(url string, handler HandlerFunc) Route {
	return r.addRoute(http.MethodDelete, url, handler)
}

// Head registers creates and returns new route with HTTP HEAD method.
func (r *router) Head(url string, handler HandlerFunc) Route {
	return r.addRoute(http.MethodHead, url, handler)
}

// Serve seaches for the right handler – and middleware – based upon the given context.
func (r *router) Serve(ctx Context) {
	if r.panicHandler != nil {
		defer func() {
			if val := recover(); val != nil {
				r.panicHandler(ctx, val)
			}
		}()
	}

	var (
		handler = r.getHandler(ctx)

		preMw  = r.filterMatchinMiddleware(ctx, MiddlewarePreRunner)
		postMw = r.filterMatchinMiddleware(ctx, MiddlewarePostRunner)
	)

	// Then simply execute the chain.
	var (
		preChain  = preMw.createChain(handler)
		postChain = postMw.createChain(func(_ Context) {})
	)
	preChain(ctx)
	postChain(ctx)
}

// ServeHTTP is the main entrypoint for every incoming HTTP requests.
// It gets a context out of the pool – resets it based upon the
// request and writer – then passes it to Serve.
// After it has been served, simply frees all the pointers then puts the context back.
func (router *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get a context out of the pool.
	ctx := router.contextPool.Get().(*context)
	ctx.Reset(w, r)

	router.Serve(ctx)

	// Release every pointer then put it back to the pool.
	// If we didnt release the all the pointers, then the GC
	// cant free the pointer until we call ctx.reset on
	// the same pointer.
	ctx.Empty()
	router.contextPool.Put(ctx)
}

// RegisterMiddlewares registers the given middlewares as PRE runner middlewares.
func (router *router) RegisterMiddlewares(middlewares ...Middleware) {
	router.appendToMiddlewares(MiddlewarePreRunner, middlewares...)
}

// RegisterPostMiddlewares registers the given middlewares as POST runner middlewares.
func (router *router) RegisterPostMiddlewares(middlewares ...Middleware) {
	router.appendToMiddlewares(MiddlewarePostRunner, middlewares...)
}

func (router *router) appendToMiddlewares(mType middlewareType, middlewares ...Middleware) {
	if len(middlewares) == 0 {
		return
	}

	if mType == MiddlewarePostRunner {
		router.middlewares[mType] = append(middlewares, router.middlewares[mType]...)

		return
	}

	router.middlewares[mType] = append(router.middlewares[mType], middlewares...)
}

func (router *router) filterMatchinMiddleware(ctx Context, mwType middlewareType) middlewares {
	mm := make([]Middleware, 0)
	for _, m := range router.middlewares[mwType] {
		if !router.areMiddlewaresEnabled && !m.IsAlwaysAllowed() {
			continue
		}

		if m.DoesMatch(ctx) {
			mm = append(mm, m)
		}
	}
	return mm
}

func getContextIdChan() contextIdChan {
	ch := make(chan uint64)
	go func() {
		var counter uint64 = 1
		for {
			ch <- counter
			counter++
		}
	}()
	return ch
}

func (r *router) addRoute(method string, url string, handler HandlerFunc) Route {
	route := newRoute(url, handler, r)

	if err := r.endpointTree.insert(method, url, route); err != nil {
		r.logger.Error(err.Error())
		return nil
	}

	return route
}

func defaultNotFoundHandler(ctx Context) {
	ctx.SendNotFound()
}

func defaultEmptyTreeHandler(ctx Context) {
	ctx.SendMethodNotAllowed()
}

func defaultBodyReader(r *http.Request) []byte {
	if r == nil {
		return nil
	}
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return b
}

func (r *router) getHandler(ctx Context) HandlerFunc {
	m := ctx.GetRequestMethod()
	// In case of HTTP OPTIONS, we use preregistered handler.
	if m == http.MethodOptions {
		if r.optionsHandler != nil {
			return r.optionsHandler
		}
		return func(_ Context) {}
	}

	route, params := r.endpointTree.find(m, ctx.GetCleanedUrl())
	if route == nil {
		return r.notFoundHandler
	}

	ctx.BindValue(routeParamsKey, params)

	return route.execute
}

func (r *router) getBodyReaderMiddleware() Middleware {
	var matcher = func(ctx Context) bool {
		if r.bodyReader == nil {
			return false
		}
		// If the content type of the request
		// is forbidden to read from eg. multipart/form-data
		// then this matcher should return false.
		contentType := ctx.GetContentType()
		for _, ct := range exceptionContentTypes {
			if strings.Contains(contentType, ct) {
				return false
			}
		}
		for _, e := range couldReadBody {
			if e == ctx.GetRequestMethod() {
				return true
			}
		}
		return false
	}

	var mw = func(ctx Context, next HandlerFunc) {
		ctx.BindValue(incomingBodyKey, r.bodyReader(ctx.GetRequest()))
		next(ctx)
	}

	return NewMiddleware(mw,
		MiddlewareWithMatchers(matcher),
		MiddlewareWithAlwaysAllowed(true),
	)
}

func getWriterPostMiddleware() Middleware {
	mw := func(ctx Context, next HandlerFunc) {
		ctx.WriteToResponseNow()
		next(ctx)
	}

	return NewMiddleware(mw, MiddlewareWithAlwaysAllowed(true))
}

func getLoggerMiddleware() Middleware {
	var mw = func(ctx Context, next HandlerFunc) {
		log := ctx.GetLog()
		ctx.Info(log.Serialize())
		next(ctx)
	}

	return NewMiddleware(mw, MiddlewareWithAlwaysAllowed(true))
}
