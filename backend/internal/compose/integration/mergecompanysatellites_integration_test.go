// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a company merge does with everything that NAMED the merged-away
// company: the enrichment it accumulated, the money owed against it, who was
// responsible for it, and the per-reader rows.
//
// The coverage census (backend/migrations) proves no table was forgotten.
// These prove the merge does the RIGHT thing with each one, which a census
// cannot see: it reads statements, not rows, so a relink that moved the wrong
// rows or dropped evidence would satisfy it completely.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedMergePair creates two companies and returns their ids, source first.
func seedMergePair(t *testing.T, e *Env, name string) (ids.CompanyID, ids.CompanyID) {
	t.Helper()
	return companyIDOf(e.SeedCompany(t, name+" Source", nil)),
		companyIDOf(e.SeedCompany(t, name+" Target", nil))
}

// TestMergeCompany_enrichmentFollowsTheSurvivor is defect 1 of #5739: the
// sidecars were stranded on the archived loser, where no read of the survivor
// reaches them.
func TestMergeCompany_enrichmentFollowsTheSurvivor(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	src, tgt := seedMergePair(t, e, "Enrichment")
	by := "human:" + e.Rep1.String()

	// A fact only the source holds, and a fact BOTH hold under one key: the
	// survivor's own answer must win, and the source's copy must not survive
	// as a second row.
	e.WsExec(t, `
		INSERT INTO company_fact (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'offering', 'service', 'CRM rollout', 'crm rollout', '', '', 1, 'human', $2)`, src, by)
	e.WsExec(t, `
		INSERT INTO company_fact (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'offering', 'service', 'Loser wording', 'shared key', '', '', 1, 'human', $2)`, src, by)
	e.WsExec(t, `
		INSERT INTO company_fact (company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'offering', 'service', 'Survivor wording', 'shared key', '', '', 1, 'human', $2)`, tgt, by)
	e.WsExec(t, `
		INSERT INTO company_profile_field (company_id, field, value, evidence_snippet, source_url, confidence, source, captured_by)
		VALUES ($1, 'icp', 'Mid-market manufacturers', '', '', 1, 'human', $2)`, src, by)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM company_fact WHERE company_id = $1`, src); n != 0 {
		t.Errorf("%d facts still point at the merged-away company", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_fact WHERE company_id = $1`, tgt); n != 2 {
		t.Errorf("survivor holds %d facts, want 2 — the source's own plus the one it already had under the shared key", n)
	}
	var wording string
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT value FROM company_fact WHERE company_id = $1 AND value_key = 'shared key'`, tgt).Scan(&wording)
	}); err != nil {
		t.Fatalf("read the contested fact: %v", err)
	}
	if wording != "Survivor wording" {
		t.Errorf("the contested fact reads %q, want the survivor's own — a merge fills a gap, it never overwrites an answer", wording)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_profile_field WHERE company_id = $1`, tgt); n != 1 {
		t.Errorf("survivor holds %d profile fields, want the source's 1 carried across", n)
	}
}

// TestMergeCompany_theMoneyFollowsTheSurvivor is defect 3 of #5739. The issue
// called it a merge failure; it is not — company_id is ON DELETE RESTRICT, and
// the merge ARCHIVES rather than deletes, so nothing ever errored. What the
// rows did was stay on a record no page returns, which reads to a finance user
// as the contract having vanished with the duplicate.
func TestMergeCompany_theMoneyFollowsTheSurvivor(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	src, tgt := seedMergePair(t, e, "Ledger")
	by := "human:" + e.Rep1.String()

	e.WsExec(t, `
		INSERT INTO contract (company_id, title, status, source, captured_by)
		VALUES ($1, 'Framework agreement', 'active', 'manual', $2)`, src, by)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM contract WHERE company_id = $1`, src); n != 0 {
		t.Errorf("%d contracts still point at the merged-away company, pinning it against erasure", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contract WHERE company_id = $1`, tgt); n != 1 {
		t.Errorf("survivor holds %d contracts, want the moved 1", n)
	}
}

// TestMergeCompany_oneColleagueKeepsTwoRoles guards the collision key. The
// unique index is (company_id, role_id, user_id) among live rows, so comparing
// without the role would read two genuinely different responsibilities as one
// and delete the source's.
func TestMergeCompany_oneColleagueKeepsTwoRoles(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	src, tgt := seedMergePair(t, e, "Responsibility")

	// The same colleague: account manager on the source, technical contact on
	// the survivor. Both are real assignments and both must survive.
	e.WsExec(t, `
		INSERT INTO record_assignment (company_id, user_id, role_id, source, captured_by)
		SELECT $1, $2, id, 'manual', $3 FROM record_role WHERE key = 'account_manager'`,
		src, e.Rep1, "human:"+e.Rep1.String())
	e.WsExec(t, `
		INSERT INTO record_assignment (company_id, user_id, role_id, source, captured_by)
		SELECT $1, $2, id, 'manual', $3 FROM record_role WHERE key = 'technical_contact'`,
		tgt, e.Rep1, "human:"+e.Rep1.String())
	// And a true duplicate: the same colleague in the SAME role on both. One
	// of these is redundant and the source's is the one to drop.
	e.WsExec(t, `
		INSERT INTO record_assignment (company_id, user_id, role_id, source, captured_by)
		SELECT $1, $2, id, 'manual', $3 FROM record_role WHERE key = 'executive_sponsor'`,
		src, e.Rep1, "human:"+e.Rep1.String())
	e.WsExec(t, `
		INSERT INTO record_assignment (company_id, user_id, role_id, source, captured_by)
		SELECT $1, $2, id, 'manual', $3 FROM record_role WHERE key = 'executive_sponsor'`,
		tgt, e.Rep1, "human:"+e.Rep1.String())

	if _, err := e.Contacts.MergeCompany(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.WsCount(t, `
		SELECT count(*) FROM record_assignment
		 WHERE company_id = $1 AND archived_at IS NULL`, tgt); n != 3 {
		t.Errorf("survivor carries %d live assignments, want 3 — both distinct roles kept, the duplicated one dropped", n)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM record_assignment a JOIN record_role r ON r.id = a.role_id
		 WHERE a.company_id = $1 AND a.archived_at IS NULL AND r.key = 'account_manager'`, tgt); n != 1 {
		t.Error("the source's account-manager assignment was lost: comparing without the role read it as a duplicate of the technical-contact one")
	}
}

// TestMergeCompany_theVATReceiptIsNeverDestroyed guards the one singleton that
// is evidence rather than cache. company_vat_check holds the consultation
// number VIES issued — the receipt that this installation asked, on that date,
// about that number — and a re-check cannot reproduce a historical one.
func TestMergeCompany_theVATReceiptIsNeverDestroyed(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	src, tgt := seedMergePair(t, e, "VAT")
	by := "human:" + e.Rep1.String()

	e.WsExec(t, `
		INSERT INTO company_vat_check (company_id, vat_number, status, consultation_number, checked_at, captured_by)
		VALUES ($1, 'DE111111111', 'valid', 'WAPIAAAAX0000001', now(), $2)`, src, by)
	e.WsExec(t, `
		INSERT INTO company_vat_check (company_id, vat_number, status, consultation_number, checked_at, captured_by)
		VALUES ($1, 'DE222222222', 'valid', 'WAPIAAAAX0000002', now(), $2)`, tgt, by)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// The survivor keeps its own receipt, and the source's is NOT deleted —
	// it rides the archived company, recoverable, rather than being destroyed
	// to make room.
	if n := e.WsCount(t, `SELECT count(*) FROM company_vat_check WHERE company_id = $1 AND consultation_number = 'WAPIAAAAX0000002'`, tgt); n != 1 {
		t.Error("the survivor's own VAT receipt did not survive the merge")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_vat_check WHERE company_id = $1`, src); n != 1 {
		t.Error("the merged-away company's VAT receipt was destroyed — a consultation number is evidence a re-check cannot reproduce")
	}
}

// TestMergeCompany_dedupePairsSurviveTheirOwnMerge guards the table whose
// constraints make it the hardest to relink: dedupe_candidate_ordered demands
// a strict left < right, and uq_dedupe_candidate_pair spans every disposition.
//
// Three shapes have to come out right at once — a pair naming a THIRD company
// re-normalizes onto the survivor whichever column the retired one sat in, and
// the pair between the two merging companies is left alone rather than
// rewritten into a self-pair the CHECK refuses.
func TestMergeCompany_dedupePairsSurviveTheirOwnMerge(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	// Deliberately unalike names. Creating a company files a fuzzy candidate
	// against any near-twin already on file, so three similarly-named
	// fixtures would arrive with rows this test did not write and cannot
	// predict — and the pair uniqueness then refuses the ones it does write.
	src := companyIDOf(e.SeedCompany(t, "Alpha Industries", nil))
	tgt := companyIDOf(e.SeedCompany(t, "Beta Holdings", nil))
	third := companyIDOf(e.SeedCompany(t, "Gamma Logistics", nil))
	by := "human:" + e.Rep1.String()

	// A pair naming the source and a third company, stored canonically — so
	// which column the retired company occupies is decided by the ids, and
	// the relink has to handle whichever one it lands in.
	e.WsExec(t, `
		INSERT INTO dedupe_candidate (entity_type, left_company_id, right_company_id, confidence, evidence, source, captured_by, disposition)
		VALUES ('company', LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), 0.9, '{}'::jsonb, 'manual', $3, 'open')`,
		src, third, by)
	// And the pair between the two companies being merged, already disposed —
	// the shape the dedupe queue writes one statement before it merges.
	e.WsExec(t, `
		INSERT INTO dedupe_candidate (entity_type, left_company_id, right_company_id, confidence, evidence, source, captured_by, disposition, disposed_at)
		VALUES ('company', LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), 0.9, '{}'::jsonb, 'manual', $3, 'merged', now())`,
		src, tgt, by)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// The third-company pair now names the survivor, still correctly ordered.
	if n := e.WsCount(t, `
		SELECT count(*) FROM dedupe_candidate
		 WHERE entity_type = 'company'
		   AND left_company_id = LEAST($1::uuid, $2::uuid)
		   AND right_company_id = GREATEST($1::uuid, $2::uuid)`, tgt, third); n != 1 {
		t.Error("the pair naming a third company did not re-normalize onto the survivor")
	}
	// The record of THIS merge is untouched: it still names the retired
	// company, because that is the fact it exists to record.
	if n := e.WsCount(t, `
		SELECT count(*) FROM dedupe_candidate
		 WHERE entity_type = 'company' AND disposition = 'merged'
		   AND (left_company_id = $1 OR right_company_id = $1)`, src); n != 1 {
		t.Error("the merged pair's own record was rewritten or dropped — it is the evidence that this merge happened")
	}
	// And nothing anywhere became a self-pair.
	if n := e.WsCount(t, `
		SELECT count(*) FROM dedupe_candidate
		 WHERE entity_type = 'company' AND left_company_id = right_company_id`); n != 0 {
		t.Errorf("%d dedupe pairs name one company at both ends", n)
	}
}

// TestMergeCompany_theAddressFills is defect 2 of #5739: the company merge
// filled legal_name, description and industry and left the address alone, so a
// survivor with a blank address stayed blank beside a retired record holding a
// full one.
func TestMergeCompany_theAddressFills(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	line1, city, country := "Hauptstrasse 1", "Berlin", "DE"
	source, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{
		DisplayName: "Addressed Source", Source: "manual",
		Address: &crmcontracts.Address{Line1: &line1, City: &city, Country: &country},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{
		DisplayName: "Blank Target", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	survivor, err := e.Contacts.MergeCompany(admin,
		companyIDOf(ids.UUID(source.Id)), companyIDOf(ids.UUID(target.Id)))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if survivor.Address == nil {
		t.Fatal("the survivor's address is still blank: a merge fills what the survivor does not hold")
	}
	if survivor.Address.City == nil || *survivor.Address.City != city {
		t.Errorf("survivor city = %v, want the filled %q", survivor.Address.City, city)
	}
	if survivor.Address.Line1 == nil || *survivor.Address.Line1 != line1 {
		t.Errorf("survivor line1 = %v, want the filled %q", survivor.Address.Line1, line1)
	}
}
