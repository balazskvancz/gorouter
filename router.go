package gorouter

import (
	ctxpkg "context"
	"sync"
)

type Router interface{}

type (
	methodTree     map[string]*tree
	HandlerFunc    func(Context)
	MiddlewareFunc func(Context, HandlerFunc)
)

type router struct {
	ctx ctxpkg.Context
	// The address where the router will be listening.
	address int

	// The tree.
	mTree methodTree

	contextPool sync.Pool
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

func New() Router {
	ctxIdChannel := getContextIdChan()

	r := &router{
		mTree: make(methodTree),
		contextPool: sync.Pool{
			New: func() any {
				return newContext(ctxIdChannel)
			},
		},
	}

	return r
}
