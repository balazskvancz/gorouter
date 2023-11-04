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
	router  *router

	chain []HandlerFunc
}

type Route interface {
	RegisterMiddlewares(...MiddlewareFunc) Route
	GetUrl() string
	execute(Context)
}

var _ Route = (*route)(nil)

func newRoute(url string, fn HandlerFunc, r *router) *route {
	return &route{
		fullUrl: url,
		chain:   []HandlerFunc{fn},
		router:  r,
	}
}

func (route *route) registerMiddleware(mw MiddlewareFunc) *route {
	if route == nil {
		return nil
	}

	if len(route.chain) == 0 {
		return route
	}

	if !route.router.routerInfo.areMiddlewaresEnabled {
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
	if route == nil {
		return nil
	}

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
	if route == nil {
		return ""
	}
	return route.fullUrl
}

func (route *route) execute(ctx Context) {
	if route != nil && len(route.chain) > 0 {
		route.chain[0](ctx)
	}
}
