// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates_test

// No domain code deletes an evidence row.
//
// consent_event says what a person was shown and what they answered.
// communication_decision says why a message was allowed to go out. Migration
// 1788529047 revokes UPDATE on both, so no statement anywhere can rewrite a
// finding — and it deliberately leaves DELETE granted, because the admin data
// reset clears a tenant by deleting from every table the catalog lists, in one
// transaction through the app role.
//
// That is a real product feature and it is why the permission cannot carry the
// whole obligation: a GRANT cannot tell the sweep from a defect. This gate
// carries the rest, by reading the tree.
//
// It is a prohibition rather than a census: the corpus is every statement in
// the tree, and the finding is any DELETE naming either table outside the
// ratified list.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// evidenceTables are the two whose rows a data subject is shown as proof.
var evidenceTables = []string{"communication_decision", "consent_event"}

// ratifiedEvidenceDeletes are the statements allowed to delete an evidence row,
// each with the reason it is not a defect.
//
// Keyed by file, because a file is what a reader opens. AssertAllMatched
// reports an entry that has gone stale, so a ratification outlives its
// statement by one test run rather than forever.
// The admin data reset is NOT here, and its absence is the finding rather than
// an omission: compose/datasweep.go deletes through an identifier built from
// the catalog, so it names no table in any literal and this matcher never sees
// it. That is the shape a permission had to cover and a reader had to be told
// about, which is why migration 1788529047 leaves DELETE granted and says so.
// privacy/erasure_consent.go is absent for the opposite reason — its decision
// arm is an UPDATE, and the DELETEs beside it name communication_basis and
// communication_suppression, which are not evidence about a send.
var ratifiedEvidenceDeletes = gatekit.Waive(map[string]string{
	"internal/modules/privacy/erasure_leadtwins.go": "The same Art. 17 erasure, for the lead " +
		"twins a promoted person leaves behind. Its decision arm is an UPDATE tombstoning the " +
		"address, exactly as erasure_consent.go's is; the DELETEs in the same CTE are " +
		"communication_basis and communication_suppression.",
})

// TestNoDomainCodeDeletesAnEvidenceRow reads every Go statement in the tree.
func TestNoDomainCodeDeletesAnEvidenceRow(t *testing.T) {
	scope := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return fileDeletesAnEvidenceRow(file)
		},
		Exempt: ratifiedEvidenceDeletes,
	}
	subjects := scope.Files(t)
	for _, parsed := range subjects {
		if ratifiedEvidenceDeletes.Waived(t, parsed.Path) {
			continue
		}
		t.Errorf("%s deletes an evidence row. A consent proof and an authorization decision are "+
			"what a data subject is SHOWN, and what a regulator reads as the controller's Art. 5(2) "+
			"record — a row the product can silently destroy is not either. Tombstone the "+
			"identifying columns instead, as privacy/erasure_consent.go does, or ratify the "+
			"statement in ratifiedEvidenceDeletes with the reason it is not a defect.", parsed.Path)
	}
	if len(subjects) == 0 {
		// Under-recognition is the one failure a prohibition must not have: it
		// reads a tree where its pattern matches nothing, reports PASS, and
		// there is no failing assertion to notice. The ratified files ARE
		// subjects, so an empty sweep means the matcher stopped working.
		t.Fatal("no file in the tree deletes an evidence row, not even the two ratified ones — " +
			"the statement matcher has stopped recognising what it is looking for")
	}
	ratifiedEvidenceDeletes.AssertAllMatched(t)
}

// fileDeletesAnEvidenceRow reports whether any SQL literal in the file deletes
// from one of the two tables.
//
// It matches the STATEMENT rather than the line, through the shared SQL reader,
// so a delete split across a multi-line string or built in a CTE is seen the
// same as a one-liner.
func fileDeletesAnEvidenceRow(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if text, ok := gatekit.LiteralText(expr); ok && deletesEvidence(text) {
			found = true
		}
		return true
	})
	return found
}

// deletesEvidence reads one SQL text for a delete naming either table.
//
// The sweep is the reason this cannot simply match "DELETE FROM <table>": it
// deletes through a sanitized identifier built from the catalog, so it names
// neither table in any literal. It is ratified by file instead, and this
// matcher catches the shape a defect actually takes — a hand-written statement.
func deletesEvidence(sql string) bool {
	lowered := strings.ToLower(sql)
	if !strings.Contains(lowered, "delete") {
		return false
	}
	for _, table := range evidenceTables {
		if strings.Contains(lowered, table) {
			return true
		}
	}
	return false
}
