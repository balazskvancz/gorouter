package gorouter

import (
	"testing"
)

type mockContext struct {
	Context
}

func TestDoesMatch(t *testing.T) {
	type middlewareFactory = func() Middleware

	type testCase struct {
		name               string
		getMiddleware      middlewareFactory
		expectedIsMatching bool
	}

	var mockMiddleware = func(_ Context) {}

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
				matcher := MiddlewareWithMatchers(func(_ Context) bool { return true })

				return NewMiddleware(mockMiddleware, matcher)
			},
			expectedIsMatching: true,
		},
		{
			name: "the middleware is not matching if there is only one matcher which is not matching",
			getMiddleware: func() Middleware {
				matcher := MiddlewareWithMatchers(func(_ Context) bool { return false })

				return NewMiddleware(mockMiddleware, matcher)
			},
			expectedIsMatching: false,
		},
		{
			name: "the middleware is matching if every matcher is matching",
			getMiddleware: func() Middleware {
				matchers := MiddlewareWithMatchers(
					func(_ Context) bool { return true },
					func(_ Context) bool { return true },
					func(_ Context) bool { return true },
				)

				return NewMiddleware(mockMiddleware, matchers)
			},
			expectedIsMatching: true,
		},
		{
			name: "the middleware is not matching if at least one matcher is not matching",
			getMiddleware: func() Middleware {
				matchers := MiddlewareWithMatchers(
					func(_ Context) bool { return true },
					func(_ Context) bool { return false },
					func(_ Context) bool { return true },
				)

				return NewMiddleware(mockMiddleware, matchers)
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
