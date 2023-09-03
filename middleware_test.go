package gorouter

import (
	"strings"
	"testing"
)

type mockContext struct {
	Context
}

func TestCreateChain(t *testing.T) {
	type middlewaresFactory func(*[]string) middlewares

	type testCase struct {
		name           string
		getMiddlewares middlewaresFactory
		expected       []string
	}

	tt := []testCase{
		{
			name: "the functions only returns the handler, if the middlewares are nil",
			getMiddlewares: func(i *[]string) middlewares {
				return nil
			},
			expected: []string{"4"},
		},
		{
			name: "the functions only returns the handler, if there are no middlewares",
			getMiddlewares: func(i *[]string) middlewares {
				return make(middlewares, 0)
			},
			expected: []string{"4"},
		},
		{
			name: "the functions creates a chain where not every mw calls next",
			getMiddlewares: func(arr *[]string) middlewares {
				var (
					mw1 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "1")
						next(ctx)
					})

					mw2 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "2")
					})

					mw3 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "3")
						next(ctx)
					})
				)

				return middlewares{mw1, mw2, mw3}
			},
			expected: []string{"1", "2"},
		},
		{
			name: "the functions creates a chain where every mw calls next",
			getMiddlewares: func(arr *[]string) middlewares {
				var (
					mw1 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "1")
						next(ctx)
					})

					mw2 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "2")
						next(ctx)
					})

					mw3 = NewMiddleware(func(ctx Context, next HandlerFunc) {
						*arr = append(*arr, "3")
						next(ctx)
					})
				)

				return middlewares{mw1, mw2, mw3}
			},
			expected: []string{"1", "2", "3", "4"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var (
				arr         = make([]string, 0)
				middlewares = tc.getMiddlewares(&arr)
				mc          = &mockContext{}

				handler = func(_ Context) {
					arr = append(arr, "4")
				}

				chainedHandler = middlewares.createChain(handler)
			)

			chainedHandler(mc)

			var (
				expected = strings.Join(tc.expected, ",")
				got      = strings.Join(arr, ",")
			)

			if expected != got {
				t.Errorf("expected execution order: [%s]; got order: [%s]\n", expected, got)
			}
		})

	}
}

func TestDoesMatch(t *testing.T) {
	type middlewareFactory = func() Middleware

	type testCase struct {
		name               string
		getMiddleware      middlewareFactory
		expectedIsMatching bool
	}

	var mockMiddleware = func(_ Context, _ HandlerFunc) {}

	tt := []testCase{
		{
			name: "the middleware is matching by default – no matcher was provided",
			getMiddleware: func() Middleware {
				return NewMiddleware(mockMiddleware)
			},
			expectedIsMatching: true,
		},
		{
			name: "the middleware is matching if there is only one matcher which is matching",
			getMiddleware: func() Middleware {
				return NewMiddleware(mockMiddleware, func(_ Context) bool { return true })
			},
			expectedIsMatching: true,
		},
		{
			name: "the middleware is not matching if there is only one matcher which is not matching",
			getMiddleware: func() Middleware {
				return NewMiddleware(mockMiddleware, func(_ Context) bool { return false })
			},
			expectedIsMatching: false,
		},
		{
			name: "the middleware is matching if every matcher is matching",
			getMiddleware: func() Middleware {
				return NewMiddleware(mockMiddleware,
					func(_ Context) bool { return true },
					func(_ Context) bool { return true },
					func(_ Context) bool { return true },
				)
			},
			expectedIsMatching: true,
		},
		{
			name: "the middleware is not matching if at least one matcher is not matching",
			getMiddleware: func() Middleware {
				return NewMiddleware(mockMiddleware,
					func(_ Context) bool { return true },
					func(_ Context) bool { return false },
					func(_ Context) bool { return true },
				)
			},
			expectedIsMatching: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mw = tc.getMiddleware()
				mc = &mockContext{}
			)

			if isMatching := mw.DoesMatch(mc); isMatching != tc.expectedIsMatching {
				t.Errorf("expected doesMatch value: %t; got: %t\n", tc.expectedIsMatching, isMatching)
			}

		})
	}
}
