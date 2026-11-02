// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestSearchMembershipUsesTheSharedDefinition(t *testing.T) {
	t.Parallel()
	scanned, shared := 0, 0
	for _, path := range handWrittenGoSources(t) {
		if !strings.HasPrefix(filepath.ToSlash(path), "internal/modules/search/") {
			continue
		}
		scanned++
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		alias := importAliasOf(file, "github.com/margince/margince/backend/internal/shared/kernel/employment")
		scope := helperScope{qualifier: alias, names: map[string]bool{employmentHelper: true}}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && calleeName(call) == "CurrentMembershipSQL" {
				shared++
			}
			return true
		})
		for _, decl := range file.Decls {
			for _, sql := range employmentStatements(decl, scope) {
				if strings.Contains(sql, employmentHelper) && strings.Contains(sql, "archived_at IS NULL") {
					t.Errorf("%s repeats current membership instead of CurrentMembershipSQL: %s", path, firstEmploymentLine(sql))
				}
			}
		}
	}
	if scanned == 0 || shared == 0 {
		t.Fatalf("short membership census: %d files, %d shared readers", scanned, shared)
	}
}

func TestSearchMembershipCensusSeesSplitPredicates(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"`SELECT 1 WHERE r.kind='employment' AND ` + employment.IsCurrentSQL(\"r.ended_at\") + ` AND r.archived_at IS NULL`",
		"`SELECT 1 WHERE r.archived_at IS NULL AND ` + employment.IsCurrentSQL(\"r.ended_at\") + ` AND r.kind='employment'`",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "probe.go", "package probe\nvar query = "+body, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, decl := range file.Decls {
			for _, sql := range employmentStatements(decl, helperScope{qualifier: "employment", names: map[string]bool{employmentHelper: true}}) {
				found = found || (strings.Contains(sql, employmentHelper) && strings.Contains(sql, "archived_at IS NULL"))
			}
		}
		if !found {
			t.Fatalf("split membership escaped census: %s", body)
		}
	}
}
