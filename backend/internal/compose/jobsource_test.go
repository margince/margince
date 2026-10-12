// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the job gates read off this package's source: the declared kinds keyed
// by the args type a call site names, and the hand-written files themselves.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// kindByGoType inverts the declared table: a call site names the args type, the
// contract is keyed by kind string, and Spec.GoType is the only thing joining
// them.
func kindByGoType() map[string]string {
	byType := map[string]string{}
	for kind, spec := range jobs.Declared() {
		byType[spec.GoType] = kind
	}
	return byType
}

// parseComposeSources parses this package's own hand-written PRODUCT files.
// The test binary runs with the package directory as its working directory, so
// the sources under gate are the ones beside this file.
//
// Test sources are excluded along with generated ones, and for the same
// reason: a gate here asks what the runner wires, and a fixture that registers
// a probe kind or schedules a pass to assert on it is not that.
func parseComposeSources(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing this package's sources: %v", err)
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_gen.go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, file)
	}
	return fset, files
}
