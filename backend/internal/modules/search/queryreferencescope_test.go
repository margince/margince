// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"
	"testing"
)

// A direct predicate on a reference column is the SAME question the traversal
// asks, so it carries the same scope. Without this a caller filters deals by a
// company they cannot open and reads the binding off which rows come back —
// masking the id afterwards makes the answer quieter, not different.
func TestAPredicateOnAReferenceCarriesTheReferencedRecordsScope(t *testing.T) {
	sql, _ := compilePlanDoc(teamReaderFor(entityDeal, entityCompany), t, `{
		"version": "v1", "target": "deal",
		"where": [{"field": "company_id", "op": "eq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd7"}]}`)
	if !strings.Contains(sql, "EXISTS (SELECT 1 FROM company ref0") {
		t.Fatalf("the predicate carries no company scope: %s", sql)
	}
	// Decided about the COMPANY row, under the guard's own alias. Rendered
	// against `t` it would decide about the deal — a visibility rule answering
	// about a different record, which is the defect and not the fix.
	if !strings.Contains(sql, "ref0.owner_id") {
		t.Fatalf("the guard's scope is not rendered against the referenced row: %s", sql)
	}
	if strings.Contains(sql, "t.owner_id") {
		t.Fatalf("the guard's scope filters the deal instead: %s", sql)
	}
}

// A project is read by every seat that holds the grant — it is an identity
// table, and its row scope renders nothing. So a predicate on `project_id`
// carries no guard: there is no row to withhold, and the EXISTS would only
// re-prove the foreign key at a subquery per row.
//
// This is the arm that keeps the fix from becoming an availability regression,
// and it is why the guard asks branchScope rather than deciding for itself
// which record types narrow.
func TestAPredicateOnAReferenceEverySeatReadsCarriesNoGuard(t *testing.T) {
	sql, _ := compilePlanDoc(teamReaderFor(entityDeal, entityProject), t, `{
		"version": "v1", "target": "deal",
		"where": [{"field": "project_id", "op": "eq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd7"}]}`)
	if strings.Contains(sql, "EXISTS (SELECT 1 FROM project") {
		t.Fatalf("a reference every seat may read still paid for a guard: %s", sql)
	}
}

// A row naming nothing has no target to be admitted to, and the guard lets it
// through.
//
// `neq` is what makes this load-bearing rather than tidy. It renders IS
// DISTINCT FROM, which is TRUE for a deal with no company at all — a caller
// asking for "not company X" is answered with those deals today. A guard
// without the null arm would drop them, which is a WRONG answer rather than a
// narrower one: the rows it removes disclose nothing about X.
func TestAReferenceThatNamesNothingSurvivesTheGuard(t *testing.T) {
	sql, _ := compilePlanDoc(teamReaderFor(entityDeal, entityCompany), t, `{
		"version": "v1", "target": "deal",
		"where": [{"field": "company_id", "op": "neq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd7"}]}`)
	if !strings.Contains(sql, `IS DISTINCT FROM`) {
		t.Fatalf("neq no longer renders IS DISTINCT FROM, so this case no longer asks what it was written to ask: %s", sql)
	}
	if !strings.Contains(sql, `t."company_id" IS NULL OR EXISTS`) {
		t.Fatalf("the guard has no null arm, so a deal with no company is dropped from an answer it belongs in: %s", sql)
	}
}

// Object RBAC refusing the record type outright is one gate earlier than the
// row scope and answers the same way: only the rows naming nothing survive. A
// caller who may not read companies at all must not learn which deals have
// one.
func TestAReferenceToARecordTypeTheCallerCannotReadAdmitsOnlyNulls(t *testing.T) {
	sql, _ := compilePlanDoc(teamReaderFor(entityDeal), t, `{
		"version": "v1", "target": "deal",
		"where": [{"field": "company_id", "op": "eq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd7"}]}`)
	if strings.Contains(sql, "EXISTS (SELECT 1 FROM company") {
		t.Fatalf("a caller with no company grant got a row-scope guard rather than a refusal: %s", sql)
	}
	if !strings.Contains(sql, `t."company_id" IS NULL`) {
		t.Fatalf("the predicate still answers about the reference: %s", sql)
	}
}

// Two references in one plan get two aliases. Sharing one would make the second
// EXISTS read the first's row, so the guard would answer about the wrong record
// while looking present — the failure that reads as a passing fix.
func TestTwoReferenceGuardsInOnePlanDoNotShareAnAlias(t *testing.T) {
	// Both references are companies — the customer and the partner — which
	// is the case that would go unnoticed: one alias, one table, and an EXISTS
	// that reads as correct while answering about the customer twice.
	sql, _ := compilePlanDoc(teamReaderFor(entityDeal, entityCompany), t, `{
		"version": "v1", "target": "deal",
		"where": [{"field": "company_id", "op": "eq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd7"},
		          {"field": "partner_company_id", "op": "eq",
		           "value": "01a08485-fac3-7543-b543-59cc991bbdd8"}]}`)
	if !strings.Contains(sql, "company ref0") || !strings.Contains(sql, "company ref1") {
		t.Fatalf("the two guards do not carry distinct aliases: %s", sql)
	}
	if !strings.Contains(sql, `ref1.id = t."partner_company_id"`) {
		t.Fatalf("the second guard does not ask about the partner: %s", sql)
	}
}
