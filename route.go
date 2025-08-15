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

type Route interface {
	Handler
	ExecuteChain(ctx Context, lastIndex uint8)
	RegisterMiddlewares(mws ...Middleware) Route
	GetUrl() string
}

var _ Route = (*route)(nil)

func newRoute(url string, fn HandlerFunc, r *router) *route {
	return &route{
		fullUrl:     url,
		handler:     fn,
		middlewares: make(map[MiddlewareType]Middlewares),
	}
}

func (route *route) registerMiddleware(m Middleware) *route {
	if route == nil {
		return nil
	}

	t := m.Type()
	route.middlewares[t] = append(route.middlewares[t], m)

	return route
}

// RegisterMiddlewares registers all the given middlewares one-by-one,
// then returns the route pointer.
func (route *route) RegisterMiddlewares(mws ...Middleware) Route {
	if route == nil {
		return nil
	}

	for _, m := range mws {
		route.registerMiddleware(m)
	}

	return route
}

func (route *route) GetUrl() string {
	if route == nil {
		return ""
	}
	return route.fullUrl
}

func (route *route) ExecuteChain(ctx Context, lastIndex uint8) {
	var (
		needToExecuteHandler = true
		last                 = lastIndex
	)

	for _, e := range route.middlewares[MiddlewarePreRunner] {
		e.Handle(ctx)

		currentIndex := ctx.GetCurrentIndex()
		if currentIndex == last {
			needToExecuteHandler = false

			break
		}

		last = currentIndex
	}

	if needToExecuteHandler {
		route.Handle(ctx)
	}

	for _, e := range route.middlewares[MiddlewarePostRunner] {
		e.Handle(ctx)
	}
}

func (route *route) Handle(ctx Context) {
	route.handler(ctx)
	ctx.Next()
}
