// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// legacyEvidenceIDs are the rows a test wrote to the legacy tables, one per table.
type legacyEvidenceIDs struct {
	fact, profile, contact ids.UUID
}

// seedLegacyEvidence writes a company fact, a company profile field and a
// contact profile field the way rows written before field_provenance held them sit.
func seedLegacyEvidence(ctx context.Context, t *testing.T, e *dedupeEnv, company ids.CompanyID, contact ids.UUID) legacyEvidenceIDs {
	t.Helper()
	seeded := legacyEvidenceIDs{fact: ids.NewV7(), profile: ids.NewV7(), contact: ids.NewV7()}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO company_fact_legacy
			  (id, company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source, captured_by, version)
			VALUES ($1, $2, 'company', 'phone', '+49 30 1234', '', '"+49 30 1234"', 'https://voltaq.test/impressum',
			        0.95, 'site_read', 'agent:deepread', 3)`, seeded.fact, company); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO company_profile_field_legacy
			  (id, company_id, field, value, evidence_snippet, source_url, confidence, source, captured_by)
			VALUES ($1, $2, 'icp', 'Energy-intensive manufacturers', '"for energy-intensive manufacturers"',
			        'https://voltaq.test/about', 0.9, 'site_read', 'agent:deepread')`, seeded.profile, company); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO contact_profile_field_legacy
			  (id, contact_id, field, value, evidence_snippet, source_ref, source, captured_by)
			VALUES ($1, $2, 'title', 'Head of Procurement', 'Head of Procurement', 'activity:x', 'capture_enrich', 'agent:enrich')`,
			seeded.contact, contact)
		return err
	}); err != nil {
		t.Fatalf("seed legacy evidence: %v", err)
	}
	return seeded
}

// legacyRowsLeft counts the rows still waiting in the three legacy tables.
func legacyRowsLeft(ctx context.Context, t *testing.T, e *dedupeEnv) int {
	t.Helper()
	var left int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM company_fact_legacy)
			+ (SELECT count(*) FROM company_profile_field_legacy) + (SELECT count(*) FROM contact_profile_field_legacy)`).Scan(&left)
	}); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	return left
}

// factRevisionOf reads the company's fact counter, zero when none was written.
func factRevisionOf(ctx context.Context, t *testing.T, e *dedupeEnv, company ids.CompanyID) int64 {
	t.Helper()
	var revision int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		revision, err = factRevision(ctx, tx, company)
		return err
	}); err != nil {
		t.Fatalf("read fact revision: %v", err)
	}
	return revision
}

func seedEvidenceSubjects(ctx context.Context, t *testing.T, e *dedupeEnv) (ids.CompanyID, ids.UUID) {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Voltaq Systems GmbH", Source: "manual"})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Lan Pham", Source: "manual"})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id)), ids.UUID(contact.Id)
}

func TestACorrectionMovesTheCompanysLegacyEvidenceFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, contact := seedEvidenceSubjects(ctx, t, e)
	seeded := seedLegacyEvidence(ctx, t, e, company, contact)

	facts, err := e.store.ListCompanyFacts(ctx, company)
	if err != nil || len(facts) != 1 || facts[0].Value != "+49 30 1234" {
		t.Fatalf("a legacy fact before its move = %+v, %v; want it read through the view", facts, err)
	}
	before := factRevisionOf(ctx, t, e, company)

	corrected := "+49 30 9999"
	if _, err := e.store.UpdateCompanyFact(ctx, company, "phone:", FactWriteInput{Value: &corrected}); err != nil {
		t.Fatalf("correct a legacy fact: %v", err)
	}

	var value string
	var version int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT value, version FROM field_provenance
			 WHERE id = $1 AND object_type = 'company' AND object_id = $2`, seeded.fact, company).Scan(&value, &version)
	}); err != nil {
		t.Fatalf("read the moved fact: %v", err)
	}
	if value != corrected || version != 4 {
		t.Errorf("the moved fact = %q at version %d, want %q at version 4: it keeps its id and version and takes the correction",
			value, version, corrected)
	}
	if left := legacyRowsLeft(ctx, t, e); left != 1 {
		t.Errorf("legacy rows left = %d, want 1: the company's two rows move and the contact's waits", left)
	}
	if after := factRevisionOf(ctx, t, e, company); after != before+1 {
		t.Errorf("fact revision %d -> %d, want one step: the move is not a change, the correction is", before, after)
	}
}

func TestTheDataTaskMovesEveryLegacyRowAndAReplayFindsNone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, contact := seedEvidenceSubjects(ctx, t, e)
	seeded := seedLegacyEvidence(ctx, t, e, company, contact)
	before := factRevisionOf(ctx, t, e, company)

	system := principal.SystemActing(ctx, "system:test")
	runSteps := func() int {
		changed := 0
		for _, legacy := range legacyEvidenceTables {
			in := datatask.BatchIn{Limit: 10}
			for {
				var out datatask.BatchOut
				if err := e.store.tx(system, func(tx pgx.Tx) error {
					var err error
					out, err = moveLegacyEvidenceBatch(system, tx, legacy, in)
					return err
				}); err != nil {
					t.Fatalf("%s batch: %v", legacy.step, err)
				}
				changed += out.Changed
				if out.Done {
					break
				}
				in.Cursor = out.Cursor
			}
		}
		return changed
	}

	if changed := runSteps(); changed != 3 {
		t.Errorf("the first run moved %d rows, want 3", changed)
	}
	if left := legacyRowsLeft(ctx, t, e); left != 0 {
		t.Errorf("legacy rows left = %d, want none", left)
	}
	var moved int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM field_provenance WHERE id = ANY($1) AND value IS NOT NULL`,
			[]ids.UUID{seeded.fact, seeded.profile, seeded.contact}).Scan(&moved)
	}); err != nil || moved != 3 {
		t.Errorf("rows in field_provenance under their legacy ids = %d, %v; want 3", moved, err)
	}
	if changed := runSteps(); changed != 0 {
		t.Errorf("a replay moved %d rows, want none", changed)
	}
	if after := factRevisionOf(ctx, t, e, company); after != before {
		t.Errorf("fact revision %d -> %d, want it unchanged: a move is not a change to the facts", before, after)
	}
	profile, err := e.store.ListCompanyProfileFields(ctx, company)
	if err != nil || len(profile) != 1 || profile[0].Value != "Energy-intensive manufacturers" {
		t.Errorf("the moved profile field = %+v, %v; want it read as before", profile, err)
	}
}

func TestADeletedCompanyTakesItsEvidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, _ := seedEvidenceSubjects(ctx, t, e)
	if _, err := e.store.CreateCompanyFact(ctx, company, FactCreateInput{
		Category: "company", Field: "founded_year", Value: "1998",
	}); err != nil {
		t.Fatalf("state a fact: %v", err)
	}
	var left int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM company WHERE id = $1`, company); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM field_provenance
			 WHERE object_type = 'company' AND object_id = $1 AND value IS NOT NULL`, company).Scan(&left)
	}); err != nil {
		t.Fatalf("delete the company: %v", err)
	}
	if left != 0 {
		t.Errorf("evidence rows left after the company's delete = %d, want none, as the typed tables cascaded", left)
	}
}
