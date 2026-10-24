// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A history table that links its audit row names who acted through that row.
//
// A table with both `changed_by` (a flat principal id) and `audit_log_id` has
// an audit row. That row holds the actor type, the represented human, the
// passport and the client. A statement that joins `app_user` on `changed_by` guesses
// from the `human:` prefix. An agent's move then reads as nobody or as the
// system. The name comes from privacy.AuditAttributionsTx instead.
//
// The subjects come from the head catalog, so a second history table that
// gains `audit_log_id` is covered at once.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// auditLinkedHistoryTables are the tables whose rows carry both a changed_by
// stamp and the link to their audit row.
func auditLinkedHistoryTables(t *testing.T) []string {
	t.Helper()
	schema := parseHeadCatalog(t)
	var out []string
	for _, name := range schema.tableNames() {
		var changedBy, linked bool
		for _, column := range schema.table(name).columns {
			changedBy = changedBy || column.name == "changed_by"
			linked = linked || column.name == "audit_log_id"
		}
		if changedBy && linked {
			out = append(out, name)
		}
	}
	return out
}

// namesActorFromUserTable reports whether one SQL statement reads a subject
// table's changed_by and joins app_user.
func namesActorFromUserTable(statement string, tables []string) (string, bool) {
	if !strings.Contains(statement, "changed_by") || !strings.Contains(statement, "app_user") {
		return "", false
	}
	for _, table := range tables {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(table) + `\b`).MatchString(statement) {
			return table, true
		}
	}
	return "", false
}

// sqlStatementsIn returns every string constant in the file, with a chain of
// `+` joined literals read as the one statement it is.
func sqlStatementsIn(file *ast.File) map[token.Pos]string {
	out := map[token.Pos]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if node.Op == token.ADD {
				var text strings.Builder
				ast.Inspect(node, func(m ast.Node) bool {
					if lit, ok := m.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if s, err := strconv.Unquote(lit.Value); err == nil {
							text.WriteString(s)
						}
					}
					return true
				})
				out[node.Pos()] = text.String()
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if s, err := strconv.Unquote(node.Value); err == nil {
					out[node.Pos()] = s
				}
			}
		}
		return true
	})
	return out
}

func TestNoStatementNamesAnAuditLinkedActorFromTheUserTable(t *testing.T) {
	t.Parallel()
	tables := auditLinkedHistoryTables(t)
	if len(tables) == 0 {
		t.Fatal("no history table carries both changed_by and audit_log_id — the head catalog was read short")
	}
	fset := token.NewFileSet()
	var findings []string
	var read int
	walkGoFiles(t, "internal", func(path string, body []byte) {
		read++
		if strings.HasPrefix(filepath.ToSlash(path), "internal/modules/privacy/") {
			return
		}
		file, err := parser.ParseFile(fset, path, body, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for pos, statement := range sqlStatementsIn(file) {
			if table, found := namesActorFromUserTable(statement, tables); found {
				findings = append(findings, fset.Position(pos).String()+": reads "+table+".changed_by and joins app_user")
			}
		}
	})
	if read < 500 {
		t.Fatalf("read only %d Go files under internal — the walk is broken, not the tree", read)
	}
	sort.Strings(findings)
	for _, finding := range findings {
		t.Errorf("%s; resolve the actor through privacy.AuditAttributionsTx on the table's audit_log_id", finding)
	}
}

func TestTheAuditLinkedActorGateSeesTheOldPhaseHistoryQuery(t *testing.T) {
	t.Parallel()
	old := `SELECT h.id, h.changed_by, u.display_name FROM project_phase_history h
		LEFT JOIN app_user u ON $1 || u.id::text = h.changed_by WHERE h.project_id = $2`
	if _, found := namesActorFromUserTable(old, []string{"project_phase_history"}); !found {
		t.Fatal("the gate misses the query the phase history used to run")
	}
	if _, found := namesActorFromUserTable(`SELECT id FROM app_user`, []string{"project_phase_history"}); found {
		t.Fatal("the gate flags a statement that reads no changed_by")
	}
	if got := auditLinkedHistoryTables(t); !slices.Contains(got, "project_phase_history") {
		t.Fatalf("subjects %v lack project_phase_history", got)
	}
}
