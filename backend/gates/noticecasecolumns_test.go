// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// One SELECT list for the notice-case row.
//
// noticeownership.go says its column list is the only one, which is a claim
// about the whole package rather than about the file it sits in — and a claim
// like that stops the next author looking. This is what makes it true.
//
// The drift it prevents is quiet: a second SELECT written for one new read
// would not fail anything the day it lands. It fails months later, when
// somebody adds a column, updates the list they can see, and one path starts
// answering with a case that is missing a field the other path has.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// wholeRowColumns is how many columns a query has to name before it counts as
// reading the whole case rather than a projection of it. It is NoticeCase's
// field count, stated here rather than reflected: the point is that adding a
// field to the struct without widening the constant is exactly the drift, and a
// reflected count would move with the struct and never notice.
const wholeRowColumns = 16

// noticeCaseSelect finds a SELECT list over privacy_notice_case.
//
// The list is captured so its width can be counted. gatekit's flattening turns
// a spliced constant into a single space, so a query built from
// noticeCaseColumns reads as `SELECT FROM privacy_notice_case` and names no
// columns at all — which is precisely how this tells the two apart without
// having to recognise the constant by name.
var noticeCaseSelect = regexp.MustCompile(`(?is)\bSELECT\b(.*?)\bFROM\s+privacy_notice_case\b`)

// TestOneSelectListSpellsTheNoticeCaseRow holds the claim in
// noticeownership.go's noticeCaseColumns doc comment.
//
// A NARROW PROJECTION IS NOT DRIFT, and the difference is what this gate has to
// get right or it becomes noise. OpenNoticeCasesDueSoonest reads five fields
// into OpenNoticeCase, a smaller struct the attention lane needs; it is not
// trying to be a case, and adding a column to the table should not change it.
// So the rule is about queries that read the WHOLE row: a list naming at least
// as many columns as NoticeCase has fields is claiming to be one, and that is
// the claim that has to go through the constant. Below that count the query is
// answering a narrower question and is left alone.
func TestOneSelectListSpellsTheNoticeCaseRow(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "consent")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the consent package: %v", err)
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, e := range entries {
		// Tests are excluded deliberately: a fixture asserting the exact shape
		// of a row it seeded is checking the database, not reading a case, and
		// forcing it through the constant would make it prove nothing.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			decl, isFunc := n.(*ast.FuncDecl)
			if !isFunc {
				return true
			}
			for _, sql := range gatekit.SQLStatementsOf(decl) {
				match := noticeCaseSelect.FindStringSubmatch(sql)
				if match == nil {
					continue
				}
				list := strings.TrimSpace(match[1])
				if list == "" || strings.Count(list, ",")+1 < wholeRowColumns {
					continue
				}
				offenders = append(offenders, e.Name()+":"+decl.Name.Name)
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Errorf("%d function(s) spell the notice-case row themselves instead of using "+
			"noticeCaseColumns:\n\t%s\n\nOne SELECT list, or the two drift the next time a column "+
			"is added: the reader that was updated sees it and the other does not, and what a case "+
			"IS then depends on which path asked.",
			len(offenders), strings.Join(offenders, "\n\t"))
	}
}
