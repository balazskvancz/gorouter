// Segment based prefix tree to store registered endpoints.
package gorouter

import (
	"errors"
	"strings"
)

var (
	errMalformedParam   error = errors.New("malformed param: usage {param-key}")
	errMalformedUrl     error = errors.New("malformed url: urls must start with /")
	errEmptyUrl         error = errors.New("empty url was provided")
	errUrlAlreadyStored error = errors.New("the given URL is already stored with the same method")

	paramStart       string = "{"
	paramEnd         string = "}"
	slash            string = "/"
	paramPlaceholder string = "/{}"
	slashRune        rune   = '/'
	paramStartByte   byte   = '{'
)

// Every registered URL replaces the inital params,
// but stores the keys and the original positions.
type param struct {
	key   string // In case of {foo} the stored key is key.
	index int    // Stores index of the segment where the key originally was.
}

type nodeValue struct {
	params []param
	route  Route
}

type node struct {
	// The stored part of the URL.
	part string
	// NodeValues for each registered method.
	values map[string]*nodeValue
	// The children of the node. In the future,
	// in case of optimization, it can change to map[char]*node,
	// where the key is the first character of the part stored by the child.
	// It would serve as an „edge” between nodes, and reduce the
	// iterations needed during the lookup.
	children []*node

	// TODO:
	//
	// Should use a uint for representing the all the methods, that
	// are registered in the subtree, by bitwise operator.
	//
	// eg.:
	// methodGet  => 1
	// methodPost => 2
	// methodPut  => 4
	//
	// If a subtree only stores endpoints registered with methodGet,
	// then the parent should hold the value 1. If another subtree
	// has endpoints registered with methodPost and with methodPut,
	// then the stored value should be 2 | 4 = 6.
	//
	// This would also reduce the lookup efficiency.
}

type foundNode struct {
	route  Route
	params map[string]string
}

func normalizeUrl(url string) (string, []param, error) {
	if url == "" {
		return "", nil, errEmptyUrl
	}

	if !strings.HasPrefix(url, slash) {
		return "", nil, errMalformedUrl
	}

	url = url[1:]

	var (
		spl    = strings.Split(url, slash)
		s      = strings.Builder{}
		params = make([]param, 0)
	)

	for i, e := range spl {
		if !strings.HasPrefix(e, paramStart) {
			s.WriteString(slash + e)
			continue
		}

		if !strings.HasSuffix(e, paramEnd) {
			return "", nil, errMalformedParam
		}

		key := e[1 : len(e)-1]

		s.WriteString(paramPlaceholder)
		params = append(params, param{key: key, index: i})
	}

	return s.String(), params, nil
}

func longestCommonPrefix(url1 string, url2 string) int {
	var (
		min   = len(url1)
		len2  = len(url2)
		index = 0
	)
	if len2 < min {
		min = len2
	}
	for index < min && url1[index] == url2[index] {
		index++
	}
	return index
}

// getOffsets the matching offsets of each string, by skipping named parameters.
func getMatchingOffsets(url1 string, url2 string) (int, int, bool) {
	var (
		len1 = len(url1)
		len2 = len(url2)

		offset1 = 0
		offset2 = 0

		doesIncludeWildcard = false
	)

	for offset1 < len1 && offset2 < len2 {
		if url1[offset1] == url2[offset2] {
			offset1++
			offset2++

			continue
		}

		// At this point, we can be sure about the fact,
		// that the two give URLs are not matching furthermore.
		if url1[offset1] != paramStartByte {
			break
		}

		doesIncludeWildcard = true

		var (
			nextSlash1Index = strings.IndexRune(url1[offset1:], slashRune)
			nextSlash2Index = strings.IndexRune(url2[offset2:], slashRune)
		)

		if nextSlash1Index == -1 {
			offset1 = len1
		} else {
			offset1 += nextSlash1Index
		}

		if nextSlash2Index == -1 {
			offset2 = len2
		} else {
			offset2 += nextSlash2Index
		}
	}

	return offset1, offset2, doesIncludeWildcard
}

func newNode() *node {
	return &node{
		children: make([]*node, 0),
		values:   make(map[string]*nodeValue),
	}
}

func (n *node) insert(method string, url string, route Route) error {
	insertUrl, params, err := normalizeUrl(url)
	if err != nil {
		return err
	}

	// In case of an empty tree, the root should store the value.
	if n.part == "" {
		n.part = insertUrl

		n.values[method] = &nodeValue{
			params: params,
			route:  route,
		}

		return nil
	}

	var (
		// Holds the pointer to the node, where have to insert the new node.
		parentCandidate *node = n
		// Holds the nodes to check iteratively.
		nodes = []*node{n}
		// Keeps track of the current part the insertable URL.
		searchUrl = insertUrl
	)

	for i := 0; i < len(nodes); i++ {
		var (
			currNode = nodes[i]
			lcp      = longestCommonPrefix(currNode.part, searchUrl)
		)

		// If there is no common prefix in this subtree,
		// then the BFS is completed at this part.
		if lcp == 0 {
			continue
		}

		// If the remaining the the URL equals
		// the to currently holded value, then we found the candidate.
		if searchUrl == currNode.part {
			parentCandidate = currNode
			searchUrl = ""

			break
		}

		// If the stored part has the same length as
		// the lcp, then we found the subtree, where
		// the search must be continued with
		// the remaining part the original URL.
		if len(currNode.part) == lcp {
			parentCandidate = currNode
			searchUrl = searchUrl[lcp:]
			nodes = append(nodes, currNode.children...)

			continue
		}

		// Otherwise a key splitting action must be carried out,
		// the parent candidate node should be the newly created one.
		newKey := currNode.part[:lcp]

		newChildNode := &node{
			part:     currNode.part[lcp:],
			values:   currNode.values,
			children: currNode.children,
		}

		currNode.part = newKey
		currNode.children = []*node{newChildNode}
		currNode.values = make(map[string]*nodeValue)
		searchUrl = searchUrl[lcp:]

		parentCandidate = currNode
	}

	// An empty URL indicates, that there is no need to
	// create a new node, since it already exists.
	// Insertion must be carried out, unless the given method
	// has been already associated with an other handler.
	if searchUrl == "" {
		if _, exists := parentCandidate.values[method]; exists {
			return errUrlAlreadyStored
		}

		parentCandidate.values[method] = &nodeValue{
			params: params,
			route:  route,
		}

		return nil
	}

	// Otherwise, a new node should be created with remaining URL part
	// and inserted into the parent candidate's children,
	parentCandidate.children = append(parentCandidate.children, &node{
		part: searchUrl,
		values: map[string]*nodeValue{
			method: &nodeValue{
				params: params,
				route:  route,
			},
		},
		children: make([]*node, 0),
	})

	return nil
}

func (n *node) find(method string, url string) (Route, pathParams) {
	var (
		nodes     = []*node{n}
		searchUrl = url

		foundNode *node = nil
	)

	for i := 0; i < len(nodes); i++ {
		var (
			currNode = nodes[i]

			// Determines how many characters are matching, despite named parameters.
			offset1, offset2, includesWildcard = getMatchingOffsets(currNode.part, searchUrl)
		)

		if offset1 != len(currNode.part) {
			continue
		}

		rem := searchUrl[offset2:]
		if rem == "" {
			foundNode = currNode

			// Exact URL matches are prioritized.
			//
			// eg.: in case of a tree holding: /api/{test} and /api/anything
			// and the input is /api/anything,
			// then the latter should be chosen.
			if !includesWildcard {
				break
			}

			continue
		}

		searchUrl = rem

		nodes = append(nodes, currNode.children...)
	}

	if foundNode == nil || foundNode.values == nil {
		return nil, nil
	}

	// The node does not support the asked method.
	v := foundNode.values[method]
	if v == nil {
		return nil, nil
	}

	params := make(pathParams)
	if len(v.params) > 0 {
		spl := strings.Split(url, "/")[1:]

		for _, e := range v.params {
			params[e.key] = spl[e.index]
		}
	}

	return v.route, params
}

/** DEBUG */
// func (n *node) debugTree() {
// dfs(n, 1)
// }

// func dfs(n *node, lvl int) {
// fmt.Printf("level: %d, stored value: %s\n", lvl, n.part)
// for _, c := range n.children {
// dfs(c, lvl+1)
// }
// }
