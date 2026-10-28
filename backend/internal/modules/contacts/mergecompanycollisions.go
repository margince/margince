// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The half of the company merge where a UNIQUENESS CONSTRAINT decides what
// moves.
//
// mergerelink_company.go carries the rows that can simply be repointed: nothing
// about them is unique per company, so an UPDATE can neither collide nor lose
// anything. Everything here is the opposite — at most one row may exist per
// company, or per (reader, company), or per pair — so a merge bringing two
// together has to answer which one survives, and the answer differs by what the
// row IS.
//
// A DERIVED row (a geocode position, an enrichment backoff) is dropped when the
// survivor has its own, because recomputing costs a job. An EVIDENCE row (the
// VIES receipt) is never destroyed, because no later work can reproduce it.
// Duplicate pairs move through moveDedupeCandidates.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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

// moveCompanyFinanceMapping re-homes every "this external customer is that
// company" mapping, live or archived, onto the survivor. A company may hold
// several accounting customers, so the loser's live mappings join the
// survivor's, and the next sync moves their invoices with them.
func moveCompanyFinanceMapping(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE finance_customer_link SET company_id = $2 WHERE company_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("move the finance customer mappings: %w", err)
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
