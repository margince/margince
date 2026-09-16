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
// Three answers appear below, and the difference between them is worth stating
// once: a DERIVED row (a geocode position, an enrichment backoff) is dropped
// when the survivor has its own, because recomputing costs a job; an EVIDENCE
// row (the VIES receipt) is never destroyed, because no later work can
// reproduce it; and a JUDGED row (a dedupe disposition) outranks an undecided
// one, because a human answered it.

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
	// end is read as the survivor. One of the two has to go — the pair is
	// unique across every disposition — and the one that goes is the one
	// carrying NO human judgement.
	//
	// Dropping the source's row unconditionally was wrong: a colleague who
	// marked source–third "not a duplicate" had that decision deleted while an
	// untouched `open` row on target–third survived, so the pair came back to
	// the queue as if nobody had ever ruled on it. A disposition is a human
	// answer and outlives an open row every time; between two rows that both
	// carry one, the survivor's own stands.
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
		                                    THEN a.right_company_id ELSE a.left_company_id END)
		        AND (b.disposition <> 'open' OR a.disposition = 'open'))`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the duplicated pairs: %w", err)
	}
	// The mirror of the rule above: where the source's row carries the only
	// judgement, the survivor's undecided row is the one that goes, so the
	// re-normalise below can move the judged one into its place.
	if _, err := tx.Exec(ctx, `
		DELETE FROM dedupe_candidate b
		 WHERE b.entity_type = 'company' AND b.disposition = 'open'
		   AND EXISTS (
		     SELECT 1 FROM dedupe_candidate a
		      WHERE a.entity_type = 'company' AND a.id <> b.id
		        AND a.disposition <> 'open'
		        AND (a.left_company_id = $1 OR a.right_company_id = $1)
		        AND LEAST(b.left_company_id, b.right_company_id) =
		            LEAST($2::uuid, CASE WHEN a.left_company_id = $1
		                                 THEN a.right_company_id ELSE a.left_company_id END)
		        AND GREATEST(b.left_company_id, b.right_company_id) =
		            GREATEST($2::uuid, CASE WHEN a.left_company_id = $1
		                                    THEN a.right_company_id ELSE a.left_company_id END))`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the survivor's undecided duplicates: %w", err)
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
