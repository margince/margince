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
	// company". The survivor IS that company now, so the instruction still
	// holds and re-offering it would be the product forgetting an answer the
	// human already gave.
	if _, err := tx.Exec(ctx, `
		INSERT INTO suggestion_dismissal (user_id, company_id, fingerprint)
		SELECT user_id, $2, fingerprint
		  FROM suggestion_dismissal WHERE company_id = $1
		ON CONFLICT (user_id, company_id, fingerprint) DO NOTHING`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("carry suggestion dismissals: %w", err)
	}
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

// moveCompanySingletons re-homes the at-most-one-per-company rows.
//
// Two different rules, because two different kinds of row:
//
// The geocode and auto-enrich states are DERIVED — a position the geocoder
// computed, a flag the capture path set. When the survivor already holds one,
// the merged-away copy is dropped: recomputing it costs a job, and keeping a
// second would need a column to say which is current.
//
// The VAT check is EVIDENCE and is never dropped. It holds the consultation
// number VIES issued — the receipt that this installation asked, on that date,
// about that number — which is what a business shows under Art. 138 to say it
// checked its counterpart. A re-check cannot reproduce a historical receipt,
// so when the survivor already holds one the merged-away row stays on its
// archived company rather than being destroyed.
func moveCompanySingletons(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	// Literal per table, for the reason carryCompanyReaderRows gives: a name
	// assembled at runtime is invisible to the coverage census.
	for _, stmt := range []string{
		`UPDATE company_geocode_state SET company_id = $2
		 WHERE company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_geocode_state WHERE company_id = $2)`,
		`UPDATE capture_auto_enrich_state SET company_id = $2
		 WHERE company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM capture_auto_enrich_state WHERE company_id = $2)`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID); err != nil {
			return fmt.Errorf("move the derived company state: %w", err)
		}
	}
	for _, stmt := range []string{
		`DELETE FROM company_geocode_state WHERE company_id = $1`,
		`DELETE FROM capture_auto_enrich_state WHERE company_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID); err != nil {
			return fmt.Errorf("retire the merged-away derived state: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE company_vat_check SET company_id = $2
		 WHERE company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_vat_check WHERE company_id = $2)`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("move the VAT check: %w", err)
	}
	return nil
}

// moveCompanyFinanceMapping re-homes the ledger's "this external customer is
// that company" mapping.
//
// The mapping is unique per (connection, company) among LIVE rows, so two
// companies mapped on one connection cannot both move. The merged-away
// mapping is left in place rather than archived, and that is deliberate:
// SyncConnection reads only unarchived mappings, so archiving here would
// silently stop syncing that external customer — an invoice that simply stops
// arriving, with nothing anywhere saying why.
//
// Leaving it costs a mapping pointing at an archived company, which the sync
// resolves to a record no page returns. That is visible and recoverable; a
// ledger that quietly goes stale is neither. Two live mappings for one company
// is a cardinality the schema does not allow today, so choosing between them
// is a product decision rather than this merge's to make.
func moveCompanyFinanceMapping(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE finance_customer_link a SET company_id = $2
		 WHERE a.company_id = $1 AND a.archived_at IS NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM finance_customer_link b
		      WHERE b.company_id = $2 AND b.connection_id = a.connection_id
		        AND b.archived_at IS NULL)`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("move the finance customer mapping: %w", err)
	}
	// An archived mapping carries no uniqueness and is the record of a
	// mapping that once held, so it follows the surviving company.
	if _, err := tx.Exec(ctx,
		`UPDATE finance_customer_link SET company_id = $2
		 WHERE company_id = $1 AND archived_at IS NOT NULL`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("repoint the retired finance mappings: %w", err)
	}
	return nil
}

// moveCompanySiteReads re-homes the reads of the company's own website.
//
// Only one read per (company, seed_url) may be in flight, so a source read
// queued against a URL the survivor is already reading is cancelled rather
// than moved — the survivor's read answers the same question.
//
// Cancelling clears status_code, status_detail and next_attempt_at together.
// site_read_outcome_shape admits those three only on 'deferred' and 'failed',
// so a deferred read — which carries all three — would violate the constraint
// if the status moved alone.
func moveCompanySiteReads(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE site_read a
		   SET status = 'cancelled', status_code = NULL, status_detail = NULL,
		       next_attempt_at = NULL
		 WHERE a.company_id = $1
		   AND a.status IN ('queued', 'deferred', 'running')
		   AND EXISTS (
		     SELECT 1 FROM site_read b
		      WHERE b.company_id = $2 AND b.seed_url = a.seed_url
		        AND b.status IN ('queued', 'deferred', 'running'))`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("cancel the colliding site reads: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE site_read SET company_id = $2 WHERE company_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("repoint the site reads: %w", err)
	}
	return nil
}

// moveCompanyScans retires the merged-away company's scan rows.
//
// A scan is a durable JOB, not a stored reading: its status is the activity
// rail's own vocabulary, its attempt number orders the rail's events, and two
// CHECK constraints tie started_at and finished_at to that status. Copying one
// onto the survivor would either collide with the reader's own row or land a
// half-built job the rail then draws — a read that is running and has never
// begun.
//
// So the row is dropped rather than carried, and nothing is lost by it: the
// scan is a per-reader reading of an account that has just changed underneath
// it, and the next open of the survivor's page queues a fresh one through
// companyscan's own writer, which re-arms an existing row and counts the
// attempt correctly. A worker still holding a claim on the dropped row finds
// it gone and its write affects nothing, which is the same outcome as a reader
// who closed the page.
func moveCompanyScans(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE company_scan a SET company_id = $2
		 WHERE a.company_id = $1
		   AND NOT EXISTS (SELECT 1 FROM company_scan b
		                    WHERE b.company_id = $2 AND b.user_id = a.user_id)`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("move the account scans: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM company_scan WHERE company_id = $1`, sourceID); err != nil {
		return fmt.Errorf("retire the merged-away account scans: %w", err)
	}
	return nil
}

// moveCompanyDedupeCandidates re-homes the duplicate pairs that name the
// merged-away company.
//
// Three things make this the hardest table here, and all three are constraints
// rather than preferences:
//
//   - dedupe_candidate_ordered requires left < right, so repointing a column
//     can break the ordering even when no duplicate exists. The rows are
//     therefore re-normalized rather than repointed in place.
//   - uq_dedupe_candidate_pair spans EVERY disposition, not just open ones, so
//     a merged or dismissed pair collides exactly like a live one.
//   - the pair being merged RIGHT NOW is marked 'merged' by the dedupe queue in
//     this same transaction, one statement before the merge runs (#1970). That
//     row must survive: deleting it leaves a queue entry claiming a merge with
//     no record of it.
//
// A pair naming both endpoints would become a self-pair and is dropped; a pair
// whose normalized form the survivor already holds is dropped as the duplicate
// it now is. Everything else re-normalizes onto the survivor.
func moveCompanyDedupeCandidates(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	// The pair between the two merging companies — including the queue's own
	// row for THIS merge, which keeps its disposition and simply stops naming
	// the retired record once the survivor absorbs it.
	if _, err := tx.Exec(ctx, `
		DELETE FROM dedupe_candidate
		 WHERE entity_type = 'company'
		   AND ((left_company_id = $1 AND right_company_id = $2)
		     OR (left_company_id = $2 AND right_company_id = $1))
		   AND disposition = 'open'`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the open self-pair: %w", err)
	}
	// A pair the survivor already has, in either orientation, once the source
	// end is read as the survivor.
	if _, err := tx.Exec(ctx, `
		DELETE FROM dedupe_candidate a
		 WHERE a.entity_type = 'company'
		   AND (a.left_company_id = $1 OR a.right_company_id = $1)
		   AND EXISTS (
		     SELECT 1 FROM dedupe_candidate b
		      WHERE b.entity_type = 'company' AND b.id <> a.id
		        AND LEAST(b.left_company_id, b.right_company_id) =
		            LEAST($2::uuid, CASE WHEN a.left_company_id = $1
		                                 THEN a.right_company_id ELSE a.left_company_id END)
		        AND GREATEST(b.left_company_id, b.right_company_id) =
		            GREATEST($2::uuid, CASE WHEN a.left_company_id = $1
		                                    THEN a.right_company_id ELSE a.left_company_id END))`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the duplicated pairs: %w", err)
	}
	// What remains re-normalizes: the survivor takes the source's place, and
	// the two ends are re-sorted so left < right still holds.
	//
	// The pair BETWEEN the two merging companies is excluded, at every
	// disposition. Re-normalizing it would name the survivor at both ends —
	// a self-pair, which dedupe_candidate_ordered refuses outright because it
	// requires a strict left < right. The open one was dropped above; a
	// disposed one stays as it is, still naming the retired company, because
	// that is precisely what it records: the merge that retired it. A row
	// saying "these two were judged the same" is not made truer by rewriting
	// both ends to one id.
	//
	// Both SET expressions read the row as it was BEFORE the update, so the
	// same CASE in each picks the same "other end" — the column the retired
	// company does NOT occupy — and LEAST/GREATEST then order the new pair.
	if _, err := tx.Exec(ctx, `
		UPDATE dedupe_candidate SET
		  left_company_id  = LEAST($2::uuid, CASE WHEN left_company_id = $1
		                                          THEN right_company_id ELSE left_company_id END),
		  right_company_id = GREATEST($2::uuid, CASE WHEN left_company_id = $1
		                                             THEN right_company_id ELSE left_company_id END)
		 WHERE entity_type = 'company'
		   AND (left_company_id = $1 OR right_company_id = $1)
		   AND CASE WHEN left_company_id = $1
		            THEN right_company_id ELSE left_company_id END <> $2`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("re-normalize the remaining pairs: %w", err)
	}
	return nil
}
