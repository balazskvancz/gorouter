package gorouter

import (
	ctxpkg "context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Router interface {
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
	methodTree       map[string]*tree
	HandlerFunc      func(Context)
	MiddlewareFunc   func(Context, HandlerFunc)
	PanicHandlerFunc func(*Context, interface{})

	routerOptionFunc func(*router)

	bodyReaderFn func(*http.Request) []byte
)

const (
	defaultAddress int = 8000
)

var (
	couldReadBody []string = []string{http.MethodPost, http.MethodPut}
)

type routerInfo struct {
	// The address where the router will be listening.
	address int

	// The maximum size of the body in case in multipart/form-data content.
	maxFormSize uint64

	//
	defaultResponseStatusCode int
}

type router struct {
	routerInfo

	// The base running context of the router, use it for cancellation.
	ctx ctxpkg.Context

	// Trees for all the registered endpoints.
	// Every HTTP Method gets a different, by default empty
	// tree, then stored in a map, where the key is the
	// method itself.
	methodTrees methodTree

	// Instead of creating a new Context for each incoming request
	// we use this pool to acquire an already initiated entity,
	// and after the initiation, we return it.
	//
	// NOTE: everytime we put one entity back to the pool, the caller
	// must empty all the attached pointers, otherwise the GC won't be
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

	// A custom function to read the body of the
	// incoming request in advance.
	bodyReader bodyReaderFn
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
func WithMaxBodySize(size uint64) routerOptionFunc {
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

// New returns a new Router instance decorated
// by the given optionFuncs.
func New(opts ...routerOptionFunc) Router {
	ctxIdChannel := getContextIdChan()

	r := &router{
		routerInfo: routerInfo{
			address:                   defaultAddress,
			defaultResponseStatusCode: defaultStatusCode,
			maxFormSize:               defaultMaxFormBodySize,
		},

		// By deafult we simply use the Background context.
		ctx: ctxpkg.Background(),

		middlewares: make(middlewareRegistry, 0),
		methodTrees: make(methodTree),

		notFoundHandler: defaultNotFoundHandler,
		optionsHandler:  nil,
		panicHandler:    nil,

		bodyReader: defaultBodyReader,
	}

	for _, o := range opts {
		o(r)
	}

	r.contextPool = sync.Pool{
		New: func() any {
			return newContext(
				ctxIdChannel,
				r.routerInfo.defaultResponseStatusCode,
				r.maxFormSize,
			)
		},
	}

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

	// If it was not disabled during by the opts,
	// then we append the middleware to preRunners.
	if r.bodyReader != nil {
		r.RegisterMiddlewares(r.getBodyReaderMiddleware())
	}

	r.RegisterPostMiddlewares(getWriterPostMiddleware())

	go func() {
		if err := server.ListenAndServe(); err != nil {
			// TODO: handle error.
			fmt.Println(err.Error())
		}
	}()

	signalChannel := make(chan os.Signal, 1)

	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	var shutdown = func() {
		fmt.Println("the router is shutting down...")
		if err := server.Shutdown(ctxpkg.Background()); err != nil {
			fmt.Println(err.Error())
		}
	}

	select {
	case <-signalChannel:
		shutdown()
	case <-ctx.Done():
		shutdown()
	}
	fmt.Println("the router is shutted down...")
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
	defer func() {
		if val := recover(); val != nil {
			fmt.Println(val)
		}
	}()

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
	ctx.reset(w, r)

	router.Serve(ctx)
	// Must be moved into postRunnerMiddlewares.

	// Release every pointer then put it back to the pool.
	// If we didnt release the all the pointers, then the GC
	// cant free the pointer until we call ctx.reset on
	// the same pointer.
	ctx.empty()
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
	router.middlewares[mType] = append(router.middlewares[mType], middlewares...)
}

func (router *router) filterMatchinMiddleware(ctx Context, mwType middlewareType) middlewares {
	mm := make([]Middleware, 0)
	for _, m := range router.middlewares[mwType] {
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
	// Get the associated tree OR create one.
	tree := func() *tree {
		if t, ok := r.methodTrees[method]; ok {
			return t
		}
		t := newTree()
		r.methodTrees[method] = t
		return t
	}()

	route := newRoute(url, handler)

	if err := tree.Insert(url, route); err != nil {
		// TODO: error handling
		fmt.Println(err.Error())
	}

	return route
}

func defaultNotFoundHandler(ctx Context) {
	ctx.SendNotFound()
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
	// In case of HTTP OPTIONS, we use preregistered handler.
	if ctx.GetRequestMethod() == http.MethodOptions {
		if r.optionsHandler != nil {
			return r.optionsHandler
		}
		return func(_ Context) {}
	}

	tree, ok := r.methodTrees[ctx.GetRequestMethod()]
	if !ok {
		return func(ctx Context) {
			ctx.SendMethodNotAllowed()
		}
	}

	routeNode := tree.Find(ctx.GetCleanedUrl())
	if routeNode == nil {
		if r.notFoundHandler != nil {
			return r.notFoundHandler
		}
		return func(_ Context) {}
	}

	var (
		params = routeNode.GetParams()
		route  = routeNode.GetValue()
	)

	// We bind the matched params to the Context with
	// the predefined key. NOTE: do not use it anywhere else!
	if len(params) > 0 {
		p := make(pathParams, len(params))
		for k, v := range params {
			p[k] = v
		}

		ctx.BindValue(routeParamsKey, p)
	}
	return route.execute
}

func (r *router) getBodyReaderMiddleware() Middleware {
	var matcher = func(ctx Context) bool {
		if r.bodyReader == nil {
			return false
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

	return NewMiddleware(mw, matcher)
}

func getWriterPostMiddleware() Middleware {
	return NewMiddleware(func(ctx Context, next HandlerFunc) {
		ctx.WriteToResponseNow()
		next(ctx)
	})
}
