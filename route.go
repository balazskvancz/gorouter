package gorouter

import (
	"strings"
)

var (
	query = '?'
)

// removeQueryParts removes the query strings from
// the given url, if there is any.
func removeQueryParts(url string) string {
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

func newroute(url string, fn HandlerFunc) *route {
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
func (route *route) RegisterMiddlewares(mws ...MiddlewareFunc) *route {
	if len(mws) == 0 {
		return route
	}

	// Have to register in reversed order.
	for i := len(mws) - 1; i >= 0; i-- {
		route.registerMiddleware(mws[i])
	}

	return route
}

func (route *route) getChain() HandlerFunc {
	return route.chain[0]
}

// getHandler returns the actual handler, which is at the end of the chain.
func (route *route) getHandler() HandlerFunc {
	return route.chain[len(route.chain)-1]
}
