package main

import (
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// newSession is the CLI entry: OS view plus the ccgo grammar engine.
func newSession(root string) *project.Session {
	return project.NewSession(root).WithEngine(ccgo.Engine{})
}
