package gorouter

type MiddlewareType string

const (
	MiddlewarePreRunner  MiddlewareType = "preRunner"
	MiddlewarePostRunner MiddlewareType = "postRunner"
)

type MiddlewareMatcherFunc func(Context) bool

type middleware struct {
	matcher         MiddlewareMatcherFunc
	handler         MiddlewareFunc
	isAlwaysAllowed bool
	mwType          MiddlewareType
}

type Middleware interface {
	Handler
	DoesMatch(Context) bool
	IsAlwaysAllowed() bool
	Type() MiddlewareType
}

type (
	Middlewares        = []Middleware
	middlewareRegistry map[MiddlewareType]Middlewares
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

// MiddlewareWithType configures the type of the new middleware.
func MiddlewareWithType(mwType MiddlewareType) MiddlewareOptionFunc {
	return func(m *middleware) {
		// TODO: validation!
		m.mwType = mwType
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
func (mw *middleware) Handle(ctx Context) {
	mw.handler(ctx)
}

// IsAlwaysAllowed returns whether a certain middleware should run
// even if the global middlewares are disabled.
func (mw *middleware) IsAlwaysAllowed() bool {
	return mw.isAlwaysAllowed
}

// Type returns the type of the middleware.
func (mw *middleware) Type() MiddlewareType {
	return mw.mwType
}
