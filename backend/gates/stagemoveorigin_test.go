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

// typedStageOrigin matches a provenance compared to one origin by hand.
var typedStageOrigin = regexp.MustCompile(`(?is)\bprovenance\s*(=|<>|!=|is\s+(not\s+)?distinct\s+from|(not\s+)?in\s*\()\s*'(imported|synced)'`)

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
	return slices.ContainsFunc(gatekit.SQLStatementsOf(file), typedStageOrigin.MatchString)
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
