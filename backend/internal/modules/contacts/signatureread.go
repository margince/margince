// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SignatureSkipReason names why the pass rejected a read of a contact's newest
// mail. All but the name gate may answer differently on a later pass. A body
// completes, and a model that misquoted once may quote right next time.
type SignatureSkipReason string

// The reasons a read is rejected. They mirror the last_skip_reason CHECK.
const (
	SignatureSkipNameGate     SignatureSkipReason = "name_gate"
	SignatureSkipEvidenceGate SignatureSkipReason = "evidence_gate"
	SignatureSkipConfidence   SignatureSkipReason = "confidence"
	SignatureSkipNarrowed     SignatureSkipReason = "narrowed"
)

// SignatureRead is what one read of a contact's newest mail yielded. The zero
// value is a clean read that found nothing to apply.
type SignatureRead struct {
	Skip    SignatureSkipReason
	Applied bool
}

// The last_outcome values, one per shape of SignatureRead.
const (
	signatureOutcomeApplied = "applied"
	signatureOutcomeNothing = "nothing_to_apply"
	signatureOutcomeSkipped = "skipped"
)

// Retried reports a rejection a later pass may overturn. A mail whose signature
// is not the contact's stays so, and most mail carries no signature at all.
func (r SignatureRead) Retried() bool {
	return r.Skip != "" && r.Skip != SignatureSkipNameGate
}

func (r SignatureRead) outcome() string {
	switch {
	case r.Skip != "":
		return signatureOutcomeSkipped
	case r.Applied:
		return signatureOutcomeApplied
	}
	return signatureOutcomeNothing
}

// SignatureReadAttempts is how many times the pass reads one mail it keeps
// rejecting. The last rejected read consumes the mail and keeps its reason.
const SignatureReadAttempts = 3

// MarkSignatureRead records one read of this mail. A clean read moves the
// watermark forward to its occurred_at, so the contact returns only with newer
// mail. A rejected read leaves it and counts a retry, until SignatureReadAttempts
// consumes the mail. Bookkeeping, so no audit or outbox row: a watermark lost
// after an apply costs one repeated read that changes nothing.
func (s *Store) MarkSignatureRead(ctx context.Context, contactID ids.ContactID, activityID ids.UUID, historyDepth int, read SignatureRead, version SignatureWork) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		retries, err := signatureRetriesTx(ctx, tx, contactID, activityID, read)
		if err != nil {
			return err
		}
		consume := !read.Retried() || retries >= SignatureReadAttempts
		// occurred_at is read from the activity rather than taken from the
		// caller: the watermark and the candidate query must compare one number.
		args := []any{contactID, activityID, historyDepth, version.Generation, version.Snapshot,
			consume, read.outcome(), string(read.Skip), retries}
		_, err = tx.Exec(ctx, fmt.Sprintf(`
            INSERT INTO contact_signature_enrich_state AS st (contact_id, activity_id, last_activity_at, history_depth, read_snapshot,
                   last_outcome, last_skip_reason, retry_activity_id, retry_count)
            SELECT $%[1]d, CASE WHEN $%[6]d THEN a.id END, CASE WHEN $%[6]d THEN a.occurred_at END, $%[3]d, NULLIF($%[5]d::text,'')::pg_snapshot,
                   $%[7]d, NULLIF($%[8]d::text, ''), CASE WHEN $%[9]d::smallint > 0 THEN a.id END, $%[9]d
              FROM activity a
			 WHERE a.id = $%[2]d AND a.restricted_at IS NULL
			ON CONFLICT (contact_id) DO UPDATE
			SET activity_id = CASE WHEN `+signatureAfterWatermarkSQL("EXCLUDED.last_activity_at", "EXCLUDED.activity_id")+` THEN EXCLUDED.activity_id ELSE st.activity_id END,
			    last_activity_at = GREATEST(st.last_activity_at,
			                                EXCLUDED.last_activity_at),
			    history_depth = EXCLUDED.history_depth,
			    attempted_at = now(),
                read_snapshot=COALESCE(EXCLUDED.read_snapshot,st.read_snapshot),
			    last_outcome = EXCLUDED.last_outcome,
			    last_skip_reason = EXCLUDED.last_skip_reason,
			    retry_activity_id = EXCLUDED.retry_activity_id,
			    retry_count = EXCLUDED.retry_count
            WHERE st.requested_generation<=$%[4]d::bigint`, signatureReadPlaceholders(len(args))...), args...)
		// No row when the activity went (erased, restricted) after the candidate
		// query. Nothing to watermark against; the next pass reads what is left.
		return err
	})
	if err != nil {
		return fmt.Errorf("contacts: recording the signature read: %w", err)
	}
	return nil
}

// signatureReadPlaceholders numbers MarkSignatureRead's arguments in order.
func signatureReadPlaceholders(count int) []any {
	numbers := make([]any, count)
	for i := range numbers {
		numbers[i] = i + 1
	}
	return numbers
}

// signatureRetriesTx is how many retried reads this mail has had, this one
// included, or 0 for a read that consumes it. The row lock holds the count while it is written.
func signatureRetriesTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, activityID ids.UUID, read SignatureRead) (int, error) {
	if !read.Retried() {
		return 0, nil
	}
	var prior int
	err := tx.QueryRow(ctx, `
		SELECT CASE WHEN retry_activity_id = $2 THEN retry_count ELSE 0 END
		  FROM contact_signature_enrich_state WHERE contact_id = $1 FOR UPDATE`,
		contactID, activityID).Scan(&prior)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("contacts: reading the signature retry count: %w", err)
	}
	return prior + 1, nil
}
