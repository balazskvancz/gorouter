package gorouter

import (
	"strings"
)

var (
	query = '?'
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
	fullUrl string

	chain []HandlerFunc
}

type Route interface {
	RegisterMiddlewares(...MiddlewareFunc) Route
	GetUrl() string
	execute(Context)
}

var _ Route = (*route)(nil)

func newRoute(url string, fn HandlerFunc) *route {
	return &route{
		fullUrl: url,
		chain:   []HandlerFunc{fn},
	}
}

func (route *route) registerMiddleware(mw MiddlewareFunc) *route {
	if len(route.chain) == 0 {
		return route
	}

	chain := route.chain

	var mwFun HandlerFunc = func(ctx Context) {
		mw(ctx, chain[0])
	}

	route.chain = append([]HandlerFunc{mwFun}, route.chain...)

	return route
}

// RegisterMiddlewares registers all the given middlewares one-by-one,
// then returns the route pointer.
func (route *route) RegisterMiddlewares(mws ...MiddlewareFunc) Route {
	if len(mws) == 0 {
		return route
	}

	// Have to register in reversed order.
	for i := len(mws) - 1; i >= 0; i-- {
		route.registerMiddleware(mws[i])
	}

	return route
}

func (route *route) GetUrl() string {
	return route.fullUrl
}

func (route *route) execute(ctx Context) {
	if len(route.chain) > 0 {
		route.chain[0](ctx)
	}
}
