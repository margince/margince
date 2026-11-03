// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var rawTaskDateReads = gatekit.Waive(map[string]string{
	"internal/modules/activities/automatictaskrepair.go:automaticTaskRepairBasis": "compares the stored value with TaskDueSQL to prove a generated-date correction; it never admits a raw deadline",
	"internal/modules/activities/homerepair.go:readHomeRepairTasksTx":             "reviewed repair snapshots the stored value so it can prove a proposed correction and detect changes",
	"internal/modules/activities/requestsettlement.go:RepliedRequests":            "settlement checks whether its own generated request reminder is pristine; it does not interpret that timestamp as a deadline",
})

var (
	activityDeadlineAlias = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+activity\s+(?:AS\s+)?("?[a-z_][a-z0-9_]*"?)`)
	unqualifiedDeadline   = regexp.MustCompile(`(?i)(?:\bSELECT\s+|\bWHERE\s+|\bAND\s+|\bOR\s+|\bORDER\s+BY\s+)due_at\b`)
)

func rawActivityDeadlines(sql string) bool {
	aliases := activityDeadlineAlias.FindAllStringSubmatch(sql, -1)
	for _, match := range aliases {
		alias := strings.Trim(match[1], `"`)
		switch strings.ToLower(alias) {
		case "where", "join", "order", "limit", "group":
			if unqualifiedDeadline.MatchString(sql) {
				return true
			}
		default:
			if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(alias) + `"?\s*\.\s*"?due_at\b`).MatchString(sql) {
				return true
			}
		}
	}
	if len(aliases) == 0 && gatekit.TableReadPattern("activity").MatchString(sql) && unqualifiedDeadline.MatchString(sql) {
		return true
	}
	return regexp.MustCompile(`(?i)\bactivity\s*\.\s*due_at\b`).MatchString(sql)
}

func TestActivityDeadlineReadersUseTheTaskPolicy(t *testing.T) {
	t.Parallel()
	defer rawTaskDateReads.AssertAllMatched(t)
	for _, source := range activityReaderScope.Files(t) {
		files := map[string]*ast.File{}
		for _, file := range parsePackageFiles(t, path.Dir(source.Path)) {
			files[gatekit.SourceFileSet().Position(file.Pos()).Filename] = file
		}
		fragments := packageFragments(files)
		for _, decl := range source.File.Decls {
			for _, sql := range answerWalkStatements(decl, fragments) {
				if !rawActivityDeadlines(sql) {
					continue
				}
				site := source.Path + ":" + declarationName(decl)
				if !rawTaskDateReads.Waived(t, site) {
					t.Errorf("%s reads a stored activity date as a deadline; use storekit.TaskDueSQL", site)
				}
			}
		}
	}
}

func TestTheDeadlineCensusFindsQualifiedAndUnqualifiedReads(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"SELECT a.due_at FROM activity a",
		"SELECT a.due_at FROM activity\n AS a WHERE a.kind='task'",
		"SELECT a.id FROM activity a ORDER BY a.due_at",
		"SELECT due_at FROM activity WHERE kind='task'",
		"SELECT due_at FROM activity",
		"SELECT activity.due_at FROM activity",
		"SELECT a.id FROM activity a WHERE a.due_at IS NULL",
	} {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), "planted.go", "package p; const query = `"+sql+"`", 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, decl := range file.Decls {
			for _, statement := range gatekit.SQLStatementsOf(decl) {
				found = found || rawActivityDeadlines(statement)
			}
		}
		if !found {
			t.Errorf("missed raw deadline read: %s", sql)
		}
	}
	if rawActivityDeadlines("SELECT c.due_at FROM contact_claim c JOIN activity a ON a.id=c.source_activity_id") {
		t.Fatal("a claim date was confused with its source activity's date")
	}
}
