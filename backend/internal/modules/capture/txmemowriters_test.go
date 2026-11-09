// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// A transaction remembers the freemail list and a seat's own identities. A
// writer of those tables that does not forget the key leaves the rest of its
// transaction deciding on the old answer. So every such writer must forget it.
func TestEveryWriterOfAMemoizedReadForgetsIt(t *testing.T) {
	memos := map[string]*regexp.Regexp{
		"freemailMemo":        regexp.MustCompile(`(?i)(INSERT INTO|UPDATE|DELETE FROM)\s+capture_freemail_domain\b`),
		"ownerIdentitiesMemo": regexp.MustCompile(`(?i)((INSERT INTO|UPDATE|DELETE FROM)\s+capture_owner_identity\b|INSERT INTO capture_connection\b|SET[^;]*\baccount_label\s*=)`),
	}
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	writers := map[string]int{}
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			body := string(source[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			for memo, write := range memos {
				if !write.MatchString(body) {
					continue
				}
				writers[memo]++
				if !strings.Contains(body, "database.Forget(tx, "+memo+")") {
					t.Errorf("%s: %s writes what %s answers and never calls database.Forget(tx, %s)", name, fn.Name.Name, memo, memo)
				}
			}
		}
	}
	// A census that finds no writer read nothing, and would pass for that.
	for memo := range memos {
		if writers[memo] == 0 {
			t.Errorf("found no writer for %s; the census reads the wrong files", memo)
		}
	}
}
