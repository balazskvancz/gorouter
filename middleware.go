package gorouter

type middlewareType string

const (
	MiddlewarePreRunner  middlewareType = "preRunner"
	MiddlewarePostRunner middlewareType = "postRunner"
)

type MiddlewareMatcherFunc func(Context) bool

type middleware struct {
	matcher         MiddlewareMatcherFunc
	handler         MiddlewareFunc
	isAlwaysAllowed bool
}

type Middleware interface {
	DoesMatch(Context) bool
	Execute(Context, HandlerFunc)
	IsAlwaysAllowed() bool
}

type (
	middlewares        []Middleware
	middlewareRegistry map[middlewareType]middlewares
)

func defaultMatcher(_ Context) bool { return true }

type MiddlewareOptionFunc func(*middleware)

// MiddlewareWithMatchers allows to configure the matchers for a given middleware.
func MiddlewareWithMatchers(matchers ...MiddlewareMatcherFunc) MiddlewareOptionFunc {
	return func(m *middleware) {
		// If there was no matcher given we use the default one.
		if len(matchers) == 0 {
			return
		}
		// Otherwise it should be matching for ALL the given matchers.
		m.matcher = func(ctx Context) bool {
			for _, matcher := range matchers {
				if isMatching := matcher(ctx); !isMatching {
					return false
				}
			}
			return true
		}
	}
}

// MiddlewareWithAlwaysAllowed configures, whether a middleware should run
// even if the middlewares are globally disallowed.
func MiddlewareWithAlwaysAllowed(isAlwaysAllowed bool) MiddlewareOptionFunc {
	return func(mw *middleware) {
		mw.isAlwaysAllowed = isAlwaysAllowed
	}
}

// NewMiddleware creates and returns a new middleware based
// upon the given MiddlewareFunc and matchers.
func NewMiddleware(handler MiddlewareFunc, opts ...MiddlewareOptionFunc) Middleware {
	mw := &middleware{
		handler: handler,
		matcher: defaultMatcher,
	}

	for _, o := range opts {
		o(mw)
	}

	return mw
}

var _ Middleware = (*middleware)(nil)

// DoesMatch returns whether a certain middleware is matching
// for a given Context.
func (mw *middleware) DoesMatch(ctx Context) bool {
	return mw.matcher(ctx)
}

// Execute executes the underlying handler with the given context
// and the Handler as next to be called.
func (mw *middleware) Execute(ctx Context, next HandlerFunc) {
	mw.handler(ctx, next)
}

// IsAlwaysAllowed returns whether a certain middleware should run
// even if the global middlewares are disabled.
func (mw *middleware) IsAlwaysAllowed() bool {
	return mw.isAlwaysAllowed
}

func (m middlewares) createChain(next HandlerFunc) HandlerFunc {
	return func(ctx Context) {
		if len(m) == 0 { // Its more verbose than checking for m == nil.
			next(ctx)
			return
		}

		var handler = reduceRight(m, func(acc HandlerFunc, curr Middleware) HandlerFunc {
			return func(ctx Context) {
				curr.Execute(ctx, acc)
			}
		}, next)

		handler(ctx)
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
