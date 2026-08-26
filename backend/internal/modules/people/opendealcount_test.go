// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// "Open deal" is a definition, not a column: a status, an archive check, and a
// currency conversion, spelled once in the 0065 rollup view. The organization
// list's count and the company page's open-pipeline tile both read it, which is
// the only reason a company page cannot show a number the list disagrees with.
//
// The drift is not a second read of the view — it is a FIRST read of `deal`.
// A count assembled here would be a second definition of open, and the two
// would disagree the first time the view's own definition moved: the fx date it
// converts at, the archive predicate, or what statuses count. Both numbers are
// on screen at once, so a reader sees the disagreement before anyone else does.
//
// Two directions, because a one-directional census misses the shape that does
// not name the column: a query that never says `open_deal_count` and counts
// open deals off the table anyway is exactly the copy this is about.

const openPipelineRollupView = "organization_open_pipeline_rollup"

// dealStatusOpen is how the open status is spelled inside a SQL literal.
const dealStatusOpen = "'open'"

func TestEveryOpenDealCountComesFromTheRollup(t *testing.T) {
	counts, dealReads := 0, 0
	for _, sql := range moduleSQLLiterals(t) {
		if strings.Contains(sql.text, "open_deal_count") {
			counts++
			if !gatekit.TableReadPattern(openPipelineRollupView).MatchString(sql.text) {
				t.Errorf("%s reads open_deal_count without reading %s:\n\n\t%s\n\n"+
					"The rollup is where open is defined. A count read from anywhere else is a "+
					"second definition, and the list and the company page show both at once.",
					sql.where, openPipelineRollupView, gatekit.FirstLineOf(sql.text))
			}
		}
		if !gatekit.TableReadPattern("deal").MatchString(sql.text) {
			continue
		}
		dealReads++
		if strings.Contains(sql.text, dealStatusOpen) {
			t.Errorf("%s reads the deal table and constrains on %s:\n\n\t%s\n\n"+
				"That assembles open here rather than taking it from %s, which is a second "+
				"definition of the word. Read the rollup, or say beside the query why this "+
				"population is not the one the rollup counts.",
				sql.where, dealStatusOpen, gatekit.FirstLineOf(sql.text), openPipelineRollupView)
		}
	}
	// Both arms need a subject. The first has one only while the module still
	// serves the count; the second only while the module still reaches the deal
	// table at all, and if it stops, the arm guarding that reach is dead weight
	// rather than a silent pass — so it says so instead of reading green.
	if counts == 0 {
		t.Errorf("no query in this module reads open_deal_count, so the arm holding it against "+
			"%s judged nothing", openPipelineRollupView)
	}
	if dealReads == 0 {
		t.Error("no query in this module reads the deal table, so the arm refusing a second " +
			"definition of open judged nothing — delete it, or find where the reads went")
	}
}

// moduleSQL is one string literal in the module's own sources, with where it is.
type moduleSQL struct {
	where string
	text  string
}

// moduleSQLLiterals returns every string literal in the module's non-test
// sources. Every literal, not the ones that look like SQL: deciding what looks
// like SQL is where a census goes blind, and a literal that is not a query
// matches neither pattern below anyway.
func moduleSQLLiterals(t *testing.T) []moduleSQL {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the module directory: %v", err)
	}
	var found []moduleSQL
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok {
				return true
			}
			text, isText := gatekit.LiteralText(lit)
			if !isText {
				return true
			}
			found = append(found, moduleSQL{
				where: fset.Position(lit.Pos()).String(), text: text,
			})
			return true
		})
	}
	if len(found) == 0 {
		t.Fatal("no string literal found in the module's sources, so this census read nothing")
	}
	return found
}
