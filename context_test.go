package gorouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type contextFactory func(*testing.T) Context
type contextFactoryWithWriter func(*testing.T, http.ResponseWriter) Context

func TestValueBinding(t *testing.T) {
	// This test is not only a unit test
	// because here we test both the functionality
	// of BindValue and GetBindedVaue aswell.

	type testCase struct {
		getContext contextFactory
		key        ContextKey
		value      any
	}

	tt := []testCase{
		// Empty context.
		{
			getContext: func(t *testing.T) Context {
				var (
					ctx = NewContext(ContextConfig{})
					rec = httptest.NewRequest(http.MethodGet, "/foo", nil)
				)

				ctx.Reset(httptest.NewRecorder(), rec)

				return ctx
			},
			key:   "empty",
			value: "bar",
		},
		// not-empty
		{
			getContext: func(t *testing.T) Context {
				var (
					ctx = NewContext(ContextConfig{})
					rec = httptest.NewRequest(http.MethodGet, "/foo", nil)
				)

				ctx.Reset(httptest.NewRecorder(), rec)

				ctx.BindValue("foo", "bar")

				return ctx
			},
			key:   "not-empty",
			value: "1",
		},
	}

	for _, tc := range tt {
		t.Run(fmt.Sprintf("test value binding for key %s", tc.key), func(t *testing.T) {
			ctx := tc.getContext(t)

			if ctx == nil {
				t.Fatal("got <nil> Context")
			}

			ctx.BindValue(tc.key, tc.value)

			value := ctx.GetBindedValue(tc.key)

			if !reflect.DeepEqual(value, tc.value) {
				t.Errorf("expected value: %v; got value: %v\n", tc.value, value)
			}
		})
	}
}

func TestGetParams(t *testing.T) {
	type testCase struct {
		name       string
		getContext contextFactory
		expected   pathParams
	}

	var mockParams pathParams = pathParams{
		"foo": "bar",
	}

	tt := []testCase{
		{
			name: "the function returns an empty map, if there is no params",
			getContext: func(t *testing.T) Context {
				var (
					ctx = NewContext(ContextConfig{})
					req = httptest.NewRequest(http.MethodGet, "/foo", nil)
				)

				ctx.Reset(nil, req)

				return ctx
			},
			expected: map[string]string{},
		},
		{
			name: "the function returns the associated params map",
			getContext: func(t *testing.T) Context {
				var (
					ctx = NewContext(ContextConfig{})
					req = httptest.NewRequest(http.MethodGet, "/foo", nil)
				)

				ctx.Reset(nil, req)

				ctx.BindValue(routeParamsKey, mockParams)

				return ctx
			},
			expected: mockParams,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.getContext(t)

			if ctx == nil {
				t.Fatal("the ctx is nil")
			}
			params := ctx.GetParams()

			if !reflect.DeepEqual(params, tc.expected) {
				t.Errorf("")
			}
		})
	}
}

func TestResponse(t *testing.T) {
	// In this tests we test all the response types for the context.
	type writefn func(Context)

	type testCase struct {
		name       string
		getContext contextFactoryWithWriter
		write      writefn

		expectedStatusCode int
		expectedBody       string
		expectedHeader     http.Header
	}

	var defaultGetCtx = func(t *testing.T, res http.ResponseWriter) Context {
		var (
			ctx = NewContext(ContextConfig{})
			req = httptest.NewRequest(http.MethodGet, "/api", nil)
		)

		ctx.Reset(res, req)

		return ctx
	}

	type testRes struct {
		Name string `json:"name"`
	}

	var resData testRes = testRes{
		Name: "test",
	}

	marshaled, _ := json.Marshal(&resData)

	tt := []testCase{
		{
			name:       "SendJson",
			getContext: defaultGetCtx,
			write: func(ctx Context) {
				ctx.SendJson(&resData)
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       string(marshaled),
			expectedHeader: http.Header{
				"Content-Type": []string{JsonContentTypeUTF8},
			},
		},
		{
			name:       "SendNotFound",
			getContext: defaultGetCtx,
			write: func(ctx Context) {
				ctx.SendNotFound()
			},
			expectedStatusCode: http.StatusNotFound,
			expectedBody:       http.StatusText(http.StatusNotFound),
			expectedHeader: http.Header{
				"Content-Type":           []string{"text/plain; charset=utf-8"},
				"X-Content-Type-Options": []string{"nosniff"},
			},
		},
		{
			name:       "SendInternalServerError",
			getContext: defaultGetCtx,
			write: func(ctx Context) {
				ctx.SendInternalServerError()
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedBody:       http.StatusText(http.StatusInternalServerError),
			expectedHeader: http.Header{
				"Content-Type":           []string{"text/plain; charset=utf-8"},
				"X-Content-Type-Options": []string{"nosniff"},
			},
		},
		{
			name:       "SendMethodNotAllowed",
			getContext: defaultGetCtx,
			write: func(ctx Context) {
				ctx.SendMethodNotAllowed()
			},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedBody:       http.StatusText(http.StatusMethodNotAllowed),
			expectedHeader: http.Header{
				"Content-Type":           []string{"text/plain; charset=utf-8"},
				"X-Content-Type-Options": []string{"nosniff"},
			},
		},
		{
			name:       "SendUnauthorized",
			getContext: defaultGetCtx,
			write: func(ctx Context) {
				ctx.SendUnauthorized()
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedBody:       http.StatusText(http.StatusUnauthorized),
			expectedHeader: http.Header{
				"Content-Type":           []string{"text/plain; charset=utf-8"},
				"X-Content-Type-Options": []string{"nosniff"},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var (
				rec = httptest.NewRecorder()
				ctx = tc.getContext(t, rec)
			)

			tc.write(ctx)

			ctx.WriteToResponseNow()

			var (
				writtenCode   = rec.Code
				writtenBody   = strings.ReplaceAll(rec.Body.String(), "\n", "")
				writtenHeader = rec.Header()
			)

			if writtenCode != tc.expectedStatusCode {
				t.Errorf("expected statusCode: %d; got: %d\n", tc.expectedStatusCode, writtenCode)
			}

			if !reflect.DeepEqual(tc.expectedBody, writtenBody) {
				t.Errorf("expected body: %s; got: %s;\n", string(tc.expectedBody), writtenBody)
			}

			if !reflect.DeepEqual(tc.expectedHeader, writtenHeader) {
				t.Errorf("expected header: %v; got: %v\n", tc.expectedHeader, writtenHeader)
			}
		})
	}
}
