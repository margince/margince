// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Nothing writes when a record entered the installation.
//
// lead, contact and deal carry entered_at: the time the row was inserted here,
// which retention asks alongside the record's own dates so an import that
// states old source-system dates is not swept on the day it arrives. The
// column is only worth anything while nothing moves it after the insert. An
// import that backdated it with created_at would bring back exactly the sweep
// it exists to stop, and the write would succeed silently.
//
// This reads the tree, so a writer added later is judged the same way.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// writesEnteredAt matches a SQL statement that assigns or inserts entered_at.
var writesEnteredAt = regexp.MustCompile(`(?is)(\bSET\b[^;]*\bentered_at\s*=|\bINSERT\s+INTO\b[^;]*\bentered_at\b)`)

func TestNothingWritesWhenARecordEnteredTheInstall(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var offences []string
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "internal/contracts/") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if writesEnteredAt.MatchString(text) {
				offences = append(offences, fset.Position(lit.Pos()).String())
			}
			return true
		})
	}
	for _, o := range offences {
		t.Errorf("%s writes entered_at. It records when a row entered this installation and is set only by "+
			"its insert default; retention reads it so an import is not swept on arrival, and a statement that "+
			"moves it (a backdate beside created_at) brings that sweep back", o)
	}
}
