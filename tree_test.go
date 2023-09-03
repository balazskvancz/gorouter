// NOTE: The actual testing for the tree eg. insert and find
// is not done here, because the rtree package alreay does this.
// Here only test the wrapper method(s).
package gorouter

import "testing"

func TestNewTree(t *testing.T) {
	t.Run("a new tree factory", func(t *testing.T) {
		if tree := newTree(); tree == nil {
			t.Errorf("expected tree not to be <nil>")
		}
	})
}
