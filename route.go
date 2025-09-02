package gorouter

import (
	"strings"
)

var (
	query rune = '?'
)

// removeQueryParts removes the query strings from
// the given url, if there is any.
func removeQueryPart(url string) string {
	idx := strings.IndexRune(url, query)
	if idx > 0 {
		return url[:idx]
	}
	return url
}

type route struct {
	fullUrl     string
	handler     HandlerFunc
	middlewares map[MiddlewareType]Middlewares
}

type ExecuteChainer interface {
	ExecuteChain(ctx Context, lastIndex uint8)
}

type Route interface {
	Handler
	ExecuteChainer
	RegisterMiddlewares(mws ...Middleware) Route
	GetUrl() string
}

var _ Route = (*route)(nil)

func newRoute(url string, fn HandlerFunc, r *router) Route {
	return &route{
		fullUrl:     url,
		handler:     fn,
		middlewares: make(map[MiddlewareType]Middlewares),
	}
}

func (r *route) registerMiddleware(m Middleware) Route {
	if r == nil {
		return nil
	}

	t := m.Type()
	r.middlewares[t] = append(r.middlewares[t], m)

	return r
}

// RegisterMiddlewares registers all the given middlewares one-by-one,
// then returns the route pointer.
func (r *route) RegisterMiddlewares(mws ...Middleware) Route {
	if r == nil {
		return nil
	}

	for _, m := range mws {
		r.registerMiddleware(m)
	}

	return r
}

func (r *route) GetUrl() string {
	if r == nil {
		return ""
	}
	return r.fullUrl
}

func (r *route) ExecuteChain(ctx Context, lastIndex uint8) {
	var (
		needToExecuteHandler = true
		last                 = lastIndex
	)

	for _, e := range r.middlewares[MiddlewarePreRunner] {
		e.Handle(ctx)

		currentIndex := ctx.GetCurrentIndex()
		if currentIndex == last {
			needToExecuteHandler = false

			break
		}

		last = currentIndex
	}

	if needToExecuteHandler {
		r.Handle(ctx)
	}

	for _, e := range r.middlewares[MiddlewarePostRunner] {
		e.Handle(ctx)
	}
}

func (r *route) Handle(ctx Context) {
	r.handler(ctx)
	ctx.Next()
}
