package main

import (
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

// newSession is the CLI entry: OS view plus the lewkit tree-sitter engine.
func newSession(root string) *project.Session {
	return project.NewSession(root).WithEngine(treesitter.Engine{})
}
