// NOTE: The actual testing for the tree eg. insert and find
// is not done here, because the rtree package alreay does this.
// Here only test the wrapper method(s).
package gorouter

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"testing"
)

func TestNormalizeUrl(t *testing.T) {
	type testCase struct {
		name  string
		input string

		output string
		params []param
		err    error
	}

	tt := []testCase{
		{
			name:   "returns errors if the url is empty",
			input:  "",
			output: "",
			params: nil,
			err:    errEmptyUrl,
		},
		{
			name:   "returns errors if the url does not start with /",
			input:  "api/foo/bar",
			output: "",
			params: nil,
			err:    errMalformedUrl,
		},
		{
			name:   "returns errors in case of malformatted param",
			input:  "/api/foo/{bar",
			output: "",
			params: nil,
			err:    errMalformedParam,
		},
		{
			name:   "returns the input if the url does not include any param",
			input:  "/api/foo/bar",
			output: "/api/foo/bar",
			params: make([]param, 0),
			err:    nil,
		},
		{
			name:   "returns the changed url",
			input:  "/api/{foo}/{bar}",
			output: "/api/{}/{}",
			params: []param{
				{key: "foo", index: 1},
				{key: "bar", index: 2},
			},
			err: nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			url, params, err := normalizeUrl(tc.input)

			if !errors.Is(tc.err, err) {
				t.Fatalf("expected error: %v; got error: %v\n", tc.err, err)
			}

			if url != tc.output {
				t.Errorf("expected url: %s; got url: %s\n", tc.output, url)
			}

			if !reflect.DeepEqual(params, tc.params) {
				t.Errorf("expected params: %v; got params: %v\n", tc.params, params)
			}
		})
	}
}

func TestGetMatchingOffsets(t *testing.T) {
	type testCase struct {
		/** Inputs. */
		url1 string
		url2 string
		/** Expected outputs. */
		offset1          int
		offset2          int
		includesWildcard bool
	}

	tt := []testCase{
		{
			url1: "/foo/foo",
			url2: "/foo/baz",

			offset1:          5,
			offset2:          5,
			includesWildcard: false,
		},
		{
			url1: "foo/foo",
			url2: "/foo/baz",

			offset1:          0,
			offset2:          0,
			includesWildcard: false,
		},
		{
			url1: "/foo/bar",
			url2: "/foo/baz",

			offset1:          7,
			offset2:          7,
			includesWildcard: false,
		},
		{
			url1: "{}",
			url2: "baz",

			offset1:          2,
			offset2:          3,
			includesWildcard: true,
		},
		{
			url1: "/{}/foo",
			url2: "/baz/foo",

			offset1:          7,
			offset2:          8,
			includesWildcard: true,
		},
		{
			url1: "/{}/foo/{}",
			url2: "/baz/foo/bar",

			offset1:          10,
			offset2:          12,
			includesWildcard: true,
		},
		{
			url1: "/{}",
			url2: "/baz/foo/bar",

			offset1:          3,
			offset2:          4,
			includesWildcard: true,
		},
	}

	for _, tc := range tt {
		t.Run(fmt.Sprintf("%s & %s == (%d, %d)", tc.url1, tc.url2, tc.offset1, tc.offset2), func(t *testing.T) {
			o1, o2, includesWildcard := getMatchingOffsets(tc.url1, tc.url2)

			if o1 != tc.offset1 {
				t.Errorf("expected offset1: %d; got offset2 :%d\n", tc.offset1, o1)
			}

			if o2 != tc.offset2 {
				t.Errorf("expected offset2: %d; got offset2 :%d\n", tc.offset2, o2)
			}

			if includesWildcard != tc.includesWildcard {
				t.Errorf("expected includesWildcard: %t; got includesWildcard: %t\n", tc.includesWildcard, includesWildcard)
			}
		})
	}
}

type mockRoute struct {
	Route
}

func TestInsert(t *testing.T) {
	type testCase struct {
		name    string
		getTree func(*testing.T) *node
		method  string
		url     string
		route   Route
		err     error
	}

	tt := []testCase{
		{
			name: "the function returns error, if the url is invalid",
			getTree: func(*testing.T) *node {
				return newNode()
			},
			url:    "foo/bar",
			method: http.MethodPost,
			route:  &mockRoute{},
			err:    errMalformedUrl,
		},
		{
			name: "the function returns error, if the param in the url is malformed",
			getTree: func(*testing.T) *node {
				return newNode()
			},
			url:    "/foo/{bar",
			method: http.MethodPost,
			route:  &mockRoute{},
			err:    errMalformedParam,
		},
		{
			name: "the function does not return an error, in case of inserting to an empty tree",
			getTree: func(*testing.T) *node {
				return newNode()
			},
			url:    "/foo/bar",
			method: http.MethodPost,
			route:  &mockRoute{},
			err:    nil,
		},
		{
			name: "the function returns an error, in case of duplicating endpoints",
			getTree: func(t *testing.T) *node {
				tree := newNode()

				if err := tree.insert(http.MethodPost, "/foo/bar", &mockRoute{}); err != nil {
					t.Fatalf("not expected to receive error: %v\n", err)
				}

				return tree
			},
			url:    "/foo/bar",
			method: http.MethodPost,
			route:  &mockRoute{},
			err:    errUrlAlreadyStored,
		},
		{
			name: "the function does not return an error, in case of inserting to a non-empty tree",
			getTree: func(t *testing.T) *node {
				tree := newNode()

				if err := tree.insert(http.MethodPost, "/foo/bar", &mockRoute{}); err != nil {
					t.Fatalf("not expected to receive error: %v\n", err)
				}

				return tree
			},
			url:    "/foo/baz",
			method: http.MethodPost,
			route:  &mockRoute{},
			err:    nil,
		},
		{
			name: "the function does not return an error, in case of inserting the same url, but registered with different methods",
			getTree: func(t *testing.T) *node {
				tree := newNode()

				if err := tree.insert(http.MethodPost, "/foo/bar", &mockRoute{}); err != nil {
					t.Fatalf("not expected to receive error: %v\n", err)
				}

				return tree
			},
			url:    "/foo/bar",
			method: http.MethodGet,
			route:  &mockRoute{},
			err:    nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tree := tc.getTree(t)

			if err := tree.insert(tc.method, tc.url, tc.route); !errors.Is(err, tc.err) {
				t.Errorf("expected error: %v; got error: %v\n", tc.err, err)
			}
		})
	}
}

func TestFind(t *testing.T) {
	type mockRoute struct {
		Route
	}

	var (
		mockRoute1 = &mockRoute{
			Route: newRoute("/api", nil, nil),
		}
		mockRoute2 = &mockRoute{
			Route: newRoute("/api/foo/bar", nil, nil),
		}
		mockRoute3 = &mockRoute{
			Route: newRoute("/api/foo/baz", nil, nil),
		}
		mockRoute4 = &mockRoute{
			Route: newRoute("/api/foo", nil, nil),
		}
	)

	type testCase struct {
		name    string
		getTree func(t *testing.T) *node

		method         string
		url            string
		expectedRoute  Route
		expectedParams pathParams
		expectedError  error
	}

	tt := []testCase{
		{
			name: "the function returns error in case of invalid method",
			getTree: func(*testing.T) *node {
				return newNode()
			},
			method:         "bad",
			url:            "/api",
			expectedRoute:  nil,
			expectedParams: nil,
			expectedError:  errInvalidMethod,
		},
		{
			name: "the function returns nil in case of empty tree",
			getTree: func(*testing.T) *node {
				return newNode()
			},
			method:         http.MethodGet,
			url:            "/api",
			expectedRoute:  nil,
			expectedParams: nil,
			expectedError:  nil,
		},
		{
			name: "the function returns the only node, in case of matching",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodGet, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodGet,
			url:            "/api",
			expectedRoute:  mockRoute1,
			expectedParams: make(pathParams),
			expectedError:  nil,
		},
		{
			name: "the function returns error, if the found route is registered with different method",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodGet,
			url:            "/api",
			expectedRoute:  nil,
			expectedParams: nil,
			expectedError:  nil,
		},
		{
			name: "the function returns the queried node, without wildcard parameters #1",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/bar", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodGet,
			url:            "/api/foo/baz",
			expectedRoute:  mockRoute3,
			expectedParams: make(pathParams),
			expectedError:  nil,
		},
		{
			name: "the function returns the queried node, without wildcard parameters #2",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/bar", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodGet,
			url:            "/",
			expectedRoute:  mockRoute1,
			expectedParams: make(pathParams),
			expectedError:  nil,
		},
		{
			name: "the function returns the queried node, with wildcard parameter #1",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/{test}", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/bar", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:        http.MethodPost,
			url:           "/api/mock-test",
			expectedRoute: mockRoute2,
			expectedParams: pathParams{
				"test": "mock-test",
			},
			expectedError: nil,
		},
		{
			name: "the function returns the queried node, with wildcard parameter #2",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/{test}", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/{test}/{second}", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/bar", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo/baz", mockRoute3); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodPost, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/api/foo", mockRoute4); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				if err := n.insert(http.MethodGet, "/", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:        http.MethodPost,
			url:           "/api/mock-test/mock-test-2",
			expectedRoute: mockRoute4,
			expectedParams: pathParams{
				"test":   "mock-test",
				"second": "mock-test-2",
			},
			expectedError: nil,
		},
		{
			name: "the function returns the queried node in favor of exact match #1",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api/{param}", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}
				if err := n.insert(http.MethodPost, "/api/exact", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodPost,
			url:            "/api/exact",
			expectedRoute:  mockRoute2,
			expectedParams: make(pathParams),
			expectedError:  nil,
		},
		{
			name: "the function returns the queried node in favor of exact match #2",
			getTree: func(t *testing.T) *node {
				n := newNode()

				if err := n.insert(http.MethodPost, "/api/exact", mockRoute2); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}
				if err := n.insert(http.MethodPost, "/api/{param}", mockRoute1); err != nil {
					t.Fatalf("err while inserting into tree: %v\n", err)
				}

				return n
			},
			method:         http.MethodPost,
			url:            "/api/exact",
			expectedRoute:  mockRoute2,
			expectedParams: make(pathParams),
			expectedError:  nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			tree := tc.getTree(t)

			route, params, err := tree.find(tc.method, tc.url)

			if !reflect.DeepEqual(route, tc.expectedRoute) {
				t.Errorf("expected route missmatched")
				fmt.Printf("expected route: %v\n", tc.expectedRoute)
				fmt.Printf("got route: %v\n", route)
			}

			if !maps.Equal(params, tc.expectedParams) {
				t.Error("expected params missmatched")
				fmt.Printf("expected params: %v\n", tc.expectedParams)
				fmt.Printf("got params: %v\n", params)
			}

			if !errors.Is(err, tc.expectedError) {
				t.Errorf("expected error: %v; got error: %v\n", tc.expectedError, err)
			}
		})
	}
}

func BenchmarkFind(b *testing.B) {
	var (
		tree = newNode()

		h1 = mockRoute{}
		h2 = mockRoute{}
		h3 = mockRoute{}
	)

	tree.insert(http.MethodPost, "/api/foo", h1)
	tree.insert(http.MethodGet, "/api/foo", h2)
	tree.insert(http.MethodPost, "/api/{id}/{action}", h2)
	tree.insert(http.MethodPost, "/api/{id}/{action}/{date}", h3)
	tree.insert(http.MethodPost, "/api/bar", h3)
	tree.insert(http.MethodGet, "/api/bar", h1)
	tree.insert(http.MethodPost, "/api/bar/baz", h2)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tree.find(http.MethodPost, "/api/1/delete/2025")
	}
}
