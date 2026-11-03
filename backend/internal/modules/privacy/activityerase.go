// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// eraseActivityContent is the activity/erase action: the record of the meeting
// stays and its content goes, attached files included.
//
// `raw` goes with `body`, because it is the original the body was parsed from.
// The counterparty and the channel identity stay: who the exchange was with is
// the record. piicoverage_test.go's retentionKeeps for `activity` declares
// that, so reversing it fails the gate.
func (s *RetentionService) eraseActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := s.refuseIfShieldedNow(ctx, tx, id); err != nil {
		return err
	}
	err := clearActivityContent(ctx, tx, id)
	if err == nil {
		// Everything the text left behind, through the call the restriction lift
		// and the controller's release make too, so the three cannot drift.
		err = s.eraser.purgeContentDerivedFrom(ctx, tx, id, theClockRanOut)
	}
	if err == nil {
		// Not in the shared helper: this arm removes an interaction the
		// relationship aggregates counted, so it re-folds them here. The lift
		// paths emit events, and the bus consumer re-folds for them.
		err = s.invalidateGraph(ctx, tx, id)
	}
	return err
}

func clearActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE activity SET body = NULL, raw = NULL, agenda_fingerprint = NULL, subject = @subject,
 language = NULL, source_author_name = NULL, reply_verdict = NULL, reply_verdict_at = NULL,
 reply_verdict_by = NULL, archived_at = coalesce(archived_at, now()) WHERE id = @id`, pgx.StrictNamedArgs{"subject": erasedActivitySubject, "id": id})
	if err != nil {
		return err
	}
	return clearActivityCustomValues(ctx, tx, []ids.UUID{id})
}
