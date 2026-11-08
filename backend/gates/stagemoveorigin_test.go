// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A stage reader names the moves another system made through the stagemove
// kernel, so a new provenance value reaches every reader at once.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// An origin is a quoted 'imported' or 'synced'. The column may carry a table
// alias, and valuation_provenance is another column, so it never matches.
const (
	stageOriginColumn  = `(?:\b\w+\.)?\bprovenance\b`
	stageOriginLiteral = `'(?:imported|synced)'`
	stageOriginCompare = `(?:=|<>|!=|is\s+(?:not\s+)?distinct\s+from)`
)

// typedStageOrigin lists each hand-typed spelling: a comparison either way
// round, an `IN` list, and an `ARRAY` under `ANY` or `ALL`.
var typedStageOrigin = []*regexp.Regexp{
	regexp.MustCompile(`(?is)` + stageOriginColumn + `\s*` + stageOriginCompare + `\s*` + stageOriginLiteral),
	regexp.MustCompile(`(?is)` + stageOriginLiteral + `\s*` + stageOriginCompare + `\s*` + stageOriginColumn),
	regexp.MustCompile(`(?is)` + stageOriginColumn + `\s*(?:not\s+)?in\s*\([^)]*` + stageOriginLiteral),
	regexp.MustCompile(`(?is)` + stageOriginColumn + `\s*(?:=|<>|!=)\s*(?:any|all)\s*\(\s*array\s*\[[^\]]*` + stageOriginLiteral),
}

// stageOriginCase opens a simple `CASE` on the column. Its `WHEN` arms run to
// the `CASE`'s own `END`, which caseBody finds past any `CASE` nested inside it.
var (
	stageOriginCase = regexp.MustCompile(`(?is)\bcase\s+` + stageOriginColumn)
	caseOrEnd       = regexp.MustCompile(`(?is)\b(?:case|end)\b`)
	stageOriginWhen = regexp.MustCompile(`(?is)\bwhen\s+` + stageOriginLiteral)
)

func TestStageReadersNameMovesMadeElsewhereThroughTheKernel(t *testing.T) {
	t.Parallel()
	scanned := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		if typesStageOrigin(file) {
			t.Errorf("%s compares a stage move's provenance to one origin by hand; use stagemove.MadeElsewhereSQL or NotMadeElsewhereSQL", path)
		}
	}
	if scanned < 100 {
		t.Fatalf("only %d files scanned; the stage origin census missed its subjects", scanned)
	}
}

func typesStageOrigin(file *ast.File) bool {
	return slices.ContainsFunc(gatekit.SQLStatementsOf(file), typesStageOriginIn)
}

// typesStageOriginIn reports a hand-typed origin anywhere in one statement.
func typesStageOriginIn(sql string) bool {
	for _, pattern := range typedStageOrigin {
		if pattern.MatchString(sql) {
			return true
		}
	}
	return caseNamesAnOrigin(sql)
}

// caseNamesAnOrigin reports a `CASE` on the column with an origin among its
// `WHEN` arms.
func caseNamesAnOrigin(sql string) bool {
	for _, open := range stageOriginCase.FindAllStringIndex(sql, -1) {
		if stageOriginWhen.MatchString(caseBody(sql[open[1]:])) {
			return true
		}
	}
	return false
}

// caseBody is the text up to the `END` that closes the `CASE` it starts
// inside. An unclosed `CASE` runs to the end of the statement.
func caseBody(rest string) string {
	depth := 0
	for _, word := range caseOrEnd.FindAllStringIndex(rest, -1) {
		if strings.EqualFold(rest[word[0]:word[1]], "case") {
			depth++
			continue
		}
		if depth == 0 {
			return rest[:word[0]]
		}
		depth--
	}
	return rest
}

func TestTheStageOriginCensusSeesHandTypedOrigins(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"distinct from imported", "var q = `SELECT 1 FROM deal_stage_history h WHERE h.provenance IS DISTINCT FROM 'imported'`", true},
		{"equals synced", "var q = `WHERE nx.provenance = 'synced'`", true},
		{"split literal", "var q = `WHERE nx.provenance = ` + `'imported'`", true},
		{"in list", "var q = `WHERE provenance IN ('imported', 'synced')`", true},
		{"in list, origin second", "var q = `WHERE x.provenance NOT IN ('recorded', 'synced')`", true},
		{"any array", "var q = `WHERE provenance = ANY(ARRAY['imported','synced'])`", true},
		{"all array, origin second", "var q = `WHERE h.provenance <> ALL (ARRAY['recorded', 'imported'])`", true},
		{"case on the column", "var q = `CASE provenance WHEN 'imported' THEN 1 ELSE 0 END`", true},
		{"case, origin in a later arm", "var q = `CASE nx.provenance WHEN 'recorded' THEN 1 WHEN 'synced' THEN 2 END`", true},
		{"reversed comparison", "var q = `WHERE 'imported' = provenance`", true},
		{"reversed distinct", "var q = `WHERE 'synced' IS DISTINCT FROM h.provenance`", true},
		{"case, origin after a nested case", "var q = `CASE provenance WHEN 'recorded' THEN CASE WHEN x THEN 1 END WHEN 'synced' THEN 2 END`", true},
		{"case on another column", "var q = `CASE provenance WHEN 'recorded' THEN 1 END, CASE reason WHEN 'imported' THEN 2 END`", false},
		{"valuation provenance", "var q = `WHERE valuation_provenance = 'imported'`", false},
		{"other column holds the origin", "var q = `WHERE won_without_contract_reason IN ('imported')`", false},
		{"kernel predicate", `var q = "WHERE " + stagemove.MadeElsewhereSQL("nx.provenance")`, false},
		{"recorded only", "var q = `WHERE h.provenance = 'recorded'`", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", "package probe\n"+tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := typesStageOrigin(file); got != tc.want {
				t.Errorf("detects hand-typed origin = %v, want %v", got, tc.want)
			}
		})
	}
}
