package gorouter

import (
	"net/http"
	"strconv"
)

func isValidStatusCode(code int) bool {
	return http.StatusText(code) != ""
}

func numIntoByteSlice(num int) []byte {
	return []byte(strconv.Itoa(num))
}
