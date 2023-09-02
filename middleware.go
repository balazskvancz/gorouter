package gorouter

type middlewareType string

const (
	MiddlewarePreRunner  middlewareType = "preRunner"
	MiddlewarePostRunner middlewareType = "postRunner"
)

type MiddlewareMatcherFunc func(Context) bool

type middleware struct {
	matcher MiddlewareMatcherFunc
	handler MiddlewareFunc
}

type Middleware interface {
	DoesMatch(Context) bool
	Execute(Context, HandlerFunc)
}

type (
	middlewares        []Middleware
	middlewareRegistry map[middlewareType]middlewares
)

func defaultMatcher(_ Context) bool { return true }

// NewMiddleware creates and returns a new middleware based
// upon the given MiddlewareFunc and matchers.
func NewMiddleware(handler MiddlewareFunc, matchers ...MiddlewareMatcherFunc) Middleware {
	matcher := func() MiddlewareMatcherFunc {
		// If there was no matcher given we use the default one.
		if len(matchers) == 0 {
			return defaultMatcher
		}
		// Otherwise it should be matching for ALL the given matchers.
		return func(ctx Context) bool {
			for _, matcher := range matchers {
				if isMatching := matcher(ctx); !isMatching {
					return false
				}
			}
			return true
		}
	}()

	return &middleware{
		handler: handler,
		matcher: matcher,
	}
}

var _ Middleware = (*middleware)(nil)

// DoesMatch returns whether a certain middleware is matching
// for a given Context.
func (mw *middleware) DoesMatch(_ Context) bool {
	return true
}

// Executes
func (mw *middleware) Execute(ctx Context, next HandlerFunc) {
	mw.handler(ctx, next)
}

func (m middlewares) createChain(next HandlerFunc) HandlerFunc {
	return func(ctx Context) {
		if len(m) == 0 {
			next(ctx)
			return
		}

		// funcSlice := make([]HandlerFunc, len(m))

		var foo = reduceRight(m, func(acc HandlerFunc, curr Middleware) HandlerFunc {
			return func(ctx Context) {
				curr.Execute(ctx, acc)
			}
		}, next)

		foo(ctx)

		// 		funcSlice[len(m)-1] = func(ctx Context) {
		// m[len(m)-1].Execute(ctx, next)
		// }

		// for i := len(m) - 2; i >= 0; i-- {
		// var idx = i
		// funcSlice[i] = func(ctx Context) {
		// m[idx].Execute(ctx, funcSlice[idx+1])
		// }
		// }

		// var nextFunc HandlerFunc = next

		// }
		// nextFunc(ctx)

		// funcSlice[0](ctx)
	}
}

type reduceFn[K, T any] func(K, T) K

func reduceRight[T, K any](arr []T, fn reduceFn[K, T], initial K) K {
	var acc K = initial
	for idx := len(arr) - 1; idx >= 0; idx-- {
		acc = fn(acc, arr[idx])
	}
	return acc
}
