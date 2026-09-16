// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Re-homing everything that points at the merged-away COMPANY.
//
// The company twin of mergerelink.go, and it exists for the reason stated
// there: a satellite this misses is not a broken merge, it is a row still
// pointing at a record no read returns. Nothing fails, and the data is simply
// gone from view — until a hard delete of the archived company takes it for
// real, because most of these carry ON DELETE CASCADE.
//
// merge_company.go keeps the references that were always here (projects,
// deals, hierarchy, the partner extension). This file carries the rest, and
// the split is by file size rather than by meaning: absorbCompanyReferences
// was already near the function ceiling.
//
// Which tables belong here is not a judgement call — companyfkcoverage
// (backend/migrations) enumerates every foreign key pointing at company and
// fails on any column this file neither moves nor declares out of scope.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkCompanySatellites moves every remaining row that names the
// merged-away company onto the survivor.
//
// Ordered cheapest-first only for readability; no statement here depends on
// another's result, because each owns a different table.
func relinkCompanySatellites(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if err := repointCompanyReferences(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := carryCompanyEnrichment(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := carryCompanyReaderRows(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := moveCompanyAssignments(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := moveCompanySingletons(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := moveCompanyFinanceMapping(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := moveCompanySiteReads(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	if err := moveCompanyScans(ctx, tx, sourceID, targetID); err != nil {
		return err
	}
	return moveCompanyDedupeCandidates(ctx, tx, sourceID, targetID)
}

// repointCompanyReferences moves the rows whose only tie to a company is the
// pointer itself: nothing here is unique per company, so a plain UPDATE can
// neither collide nor lose a row.
//
// The money rows (contract, finance_invoice, finance_payment) are the reason
// this is not cosmetic. They are ON DELETE RESTRICT, so a row left on the
// merged-away company pins that company in place forever: the archived record
// can never be erased, and the invoice reads to a finance user as having
// vanished with the duplicate.
func repointCompanyReferences(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	for _, stmt := range []string{
		`UPDATE contract SET company_id = $2 WHERE company_id = $1`,
		`UPDATE finance_invoice SET company_id = $2 WHERE company_id = $1`,
		`UPDATE finance_payment SET company_id = $2 WHERE company_id = $1`,
		`UPDATE commission_entry SET partner_company_id = $2 WHERE partner_company_id = $1`,
		`UPDATE company_domain_disposition SET company_id = $2 WHERE company_id = $1`,
		`UPDATE sdr_handoff SET company_id = $2 WHERE company_id = $1`,
		// The resolution caches. Each says "this outside thing turned out to be
		// that company", and the answer is still true of the survivor — it is
		// the same company, under one record now. Left behind, the next read
		// resolves to a record no page returns and the match silently stops
		// counting.
		`UPDATE linkedin_connection SET matched_company_id = $2 WHERE matched_company_id = $1`,
		`UPDATE offer SET buyer_company_id = $2 WHERE buyer_company_id = $1`,
		`UPDATE signal SET resolved_company_id = $2 WHERE resolved_company_id = $1`,
		`UPDATE signal_resolution SET matched_company_id = $2 WHERE matched_company_id = $1`,
		`UPDATE signal_thread_scan SET resolved_company_id = $2 WHERE resolved_company_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID); err != nil {
			return fmt.Errorf("repoint company references: %w", err)
		}
	}
	return nil
}

// carryCompanyEnrichment moves what the product LEARNED about the company:
// the facts a site read extracted, the profile fields behind them, and the
// per-lane technical reading.
//
// Same rule and same reason as contact_profile_field in mergerelink.go: the
// survivor's own answer wins, the merged-away copy fills only what the
// survivor never answered, and every provenance column travels WITH the row
// rather than taking its default — a re-dated fact would outrank the
// company's own later evidence.
//
// Each conflict target NAMES the uniqueness it relies on, so a future
// constraint cannot silently start swallowing a different collision here.
func carryCompanyEnrichment(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO company_fact
		  (company_id, category, field, value_key, value, evidence_snippet, source_url,
		   confidence, source, captured_by, site_read_id, captured_at, retrieved_at,
		   verified_at, verified_by)
		SELECT $2, category, field, value_key, value, evidence_snippet, source_url,
		       confidence, source, captured_by, site_read_id, captured_at, retrieved_at,
		       verified_at, verified_by
		  FROM company_fact WHERE company_id = $1
		ON CONFLICT (company_id, category, field, value_key) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry company facts: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM company_fact WHERE company_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away company facts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO company_profile_field
		  (company_id, field, value, evidence_snippet, source_url, confidence, source,
		   captured_by, captured_at, retrieved_at, verified_at, verified_by)
		SELECT $2, field, value, evidence_snippet, source_url, confidence, source,
		       captured_by, captured_at, retrieved_at, verified_at, verified_by
		  FROM company_profile_field WHERE company_id = $1
		ON CONFLICT (company_id, field) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry company profile fields: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM company_profile_field WHERE company_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away company profile fields: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO company_technical_state
		  (company_id, lane, attempts, last_outcome, last_success_at, next_attempt_at)
		SELECT $2, lane, attempts, last_outcome, last_success_at, next_attempt_at
		  FROM company_technical_state WHERE company_id = $1
		ON CONFLICT (company_id, lane) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry the technical reading: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM company_technical_state WHERE company_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away technical reading: %w", err)
	}
	return nil
}

// carryCompanyReaderRows moves the per-reader rows: the brief, the dossier,
// the growth reading, and what a reader chose not to be asked again.
//
// These are keyed per (reader, company) rather than per company, so the
// collision is per reader too: a reader holding a row on BOTH companies keeps
// their own reading of the survivor, and the merged-away one is dropped
// instead of blending two readers' answers into one row neither wrote.
//
// company_scan is deliberately NOT here — it is a durable job, not a reading,
// and moveCompanyScans retires it.
func carryCompanyReaderRows(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	// Spelled out per table rather than looped over their names. The coverage
	// census reads THIS source to decide which tables the merge touches, and a
	// table name assembled at runtime is invisible to it — a statement built by
	// concatenation would move the rows and still read as forgotten.
	for _, stmt := range []string{
		`UPDATE company_brief a SET company_id = $2
		 WHERE a.company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_brief b
		                    WHERE b.company_id = $2 AND b.user_id = a.user_id)`,
		`UPDATE company_dossier a SET company_id = $2
		 WHERE a.company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_dossier b
		                    WHERE b.company_id = $2 AND b.user_id = a.user_id)`,
		`UPDATE company_growth_fit a SET company_id = $2
		 WHERE a.company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_growth_fit b
		                    WHERE b.company_id = $2 AND b.user_id = a.user_id)`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID); err != nil {
			return fmt.Errorf("carry the per-reader rows: %w", err)
		}
	}
	// The retirements name the source alone, so they carry one parameter
	// rather than two.
	for _, stmt := range []string{
		`DELETE FROM company_brief WHERE company_id = $1`,
		`DELETE FROM company_dossier WHERE company_id = $1`,
		`DELETE FROM company_growth_fit WHERE company_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID); err != nil {
			return fmt.Errorf("retire the merged-away per-reader rows: %w", err)
		}
	}
	// A dismissal is a reader saying "stop offering me this about this
	// company", and it is NOT carried — the fingerprint it is keyed by is
	// computed over the company's own id, so a row moved to the survivor
	// hashes differently from anything the survivor will ever be offered and
	// could never match again. Carrying it would move a row that cannot work.
	//
	// So the merged-away company's dismissals are retired with it. The cost is
	// real and bounded: a reader who dismissed a suggestion about the retired
	// company may be offered the equivalent one about the survivor, once, and
	// dismissing it again sticks. Re-deriving the fingerprints belongs to the
	// suggestion engine that defines them, not to the merge.
	if _, err := tx.Exec(ctx,
		`DELETE FROM suggestion_dismissal WHERE company_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away dismissals: %w", err)
	}
	return nil
}

// moveCompanyAssignments re-homes who is responsible for the company.
//
// The unique key is (company_id, role_id, user_id) — the ROLE is part of it,
// and live rows only. So the collision is not "this colleague is already on
// the survivor": it is "this colleague already holds THIS ROLE on the
// survivor". One seat owning the source as account manager and the survivor as
// technical lead is two distinct responsibilities, and comparing without the
// role would silently delete one of them.
//
// Archived assignments carry no uniqueness at all and repoint untouched:
// they are the record of who used to be responsible, and that history belongs
// with the surviving company.
func moveCompanyAssignments(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM record_assignment a
		 WHERE a.company_id = $1 AND a.archived_at IS NULL
		   AND EXISTS (
		     SELECT 1 FROM record_assignment b
		      WHERE b.company_id = $2 AND b.role_id = a.role_id
		        AND b.archived_at IS NULL
		        AND b.user_id IS NOT DISTINCT FROM a.user_id
		        AND b.team_id IS NOT DISTINCT FROM a.team_id)`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the duplicated assignments: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE record_assignment SET company_id = $2 WHERE company_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("repoint the assignments: %w", err)
	}
	return nil
}
