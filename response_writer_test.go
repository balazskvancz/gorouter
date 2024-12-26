package gorouter

import (
	"reflect"
	"testing"
)

func TestCreateStatusLine(t *testing.T) {
	type testCase struct {
		name     string
		version  httpVersion
		code     int
		expected []byte
	}

	tt := []testCase{
		{
			name:     "the functions successfully creates status line with status 200",
			version:  version10,
			code:     200,
			expected: []byte("HTTP/1.0 200 OK\r\n"),
		},
		{
			name:     "the functions successfully creates status line with status 400",
			version:  version11,
			code:     400,
			expected: []byte("HTTP/1.1 400 Bad Request\r\n"),
		},
		{
			name:     "the functions successfully creates status line with status 418",
			version:  version11,
			code:     418,
			expected: []byte("HTTP/1.1 418 I'm a teapot\r\n"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if got := createStatusLine(tc.version, tc.code); !reflect.DeepEqual(got.Bytes(), tc.expected) {
				t.Errorf("expected: %s; got: %s", string(tc.expected), got.String())
			}
		})
	}
}

func BenchmarkCreateStatusLine(b *testing.B) {
	for i := 0; i < b.N; i++ {
		createStatusLine(version11, 301)
	}
}
