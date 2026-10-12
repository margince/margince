// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

// The unit's files sit in two packages, and only a file that imports the
// extension package can name a published handler type. A chain may run
// through a file that does not import it, and may cross packages.
func TestCollectHandlerAliasesFollowsChainsAndSkipsLookalikes(t *testing.T) {
	sources := map[string][]string{
		"x": {`package x

import ext "github.com/margince/margince/backend/pkg/extension"

type Tool = ext.ToolHandler
type Job = (ext.JobHandler)
type Defined ext.InboundHandler
type NotAHandler = ext.Extension
type (
	Grouped = ext.InboundHandler
	Plain   int
)
`, `package x

import "other/extension"

type Elsewhere = extension.ToolHandler
type Tool2 = (Tool)
`},
		"y": {`package y

type Tool3 = Tool2
type Job2 = Job
type FromDefined = Defined
`},
	}
	pkgs := map[string][]*ast.File{}
	fset := token.NewFileSet()
	for pkg, files := range sources {
		for _, src := range files {
			f, err := parser.ParseFile(fset, pkg+".go", src, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", pkg, err)
			}
			pkgs[pkg] = append(pkgs[pkg], f)
		}
	}

	got := collectHandlerAliases(pkgs, extensionPkgPath)

	want := map[string]map[string]bool{
		"ToolHandler":    {"Tool": true, "Tool2": true, "Tool3": true},
		"JobHandler":     {"Job": true, "Job2": true},
		"InboundHandler": {"Grouped": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
}
