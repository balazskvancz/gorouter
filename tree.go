package gorouter

import "github.com/balazskvancz/rtree"

type tree struct {
	*rtree.Tree[*route]
}

func newTree() *tree {
	return &tree{
		Tree: rtree.New[*route](),
	}
}
