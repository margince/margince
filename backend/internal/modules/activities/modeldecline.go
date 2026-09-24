// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A message every rung of a sweep's ladder declined — a safety filter
// withholding the answer, a validator refusing every reply — is recorded as
// declined, one stamp per question, and that sweep's backlog leaves it out. Left
// unstamped it would be the oldest row in the backlog forever, so every tick
// would re-send it and split the batch it rode in into single calls.
//
// The stamp is bookkeeping about the sweep, not a judgement of the message: it
// states no verdict, so it mints no audit entry and no event, the posture
// SetCaptureLabel takes for the label beside it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MarkCaptureLabelDeclined takes one message out of the classify backlog because
// the models declined to label it, reporting whether this write stamped it.
func (s *Store) MarkCaptureLabelDeclined(ctx context.Context, id ids.UUID) (bool, error) {
	return s.markDeclined(ctx, id, "capture_label_declined_at", "capture_label")
}

// MarkOwedVerdictDeclined takes one message out of the owed backlog and the
// re-judge sweep because the models declined to judge it. A verdict already on
// the row stands: declining to re-judge it is not a judgement that it was wrong.
func (s *Store) MarkOwedVerdictDeclined(ctx context.Context, id ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	return s.markDeclined(ctx, id, "owed_verdict_declined_at", "owed_verdict")
}

// markDeclined stamps column once; `column IS NULL` is the CAS, so a second
// pass that reached the same message first leaves its stamp standing. The hold
// and archive clauses are what the backlogs test: activity_refuse_restricted_mutation
// rejects any write to a held row, and an archived row is out of every backlog
// already, so neither may fail or be touched by the sweep.
func (s *Store) markDeclined(ctx context.Context, id ids.UUID, column, question string) (applied bool, err error) {
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		stamp := pgx.Identifier{column}.Sanitize()
		tag, err := tx.Exec(ctx, `
			UPDATE activity SET `+stamp+` = now()
			WHERE id = $1 AND `+stamp+` IS NULL AND restricted_at IS NULL AND archived_at IS NULL`, id)
		if err != nil {
			return fmt.Errorf("activities: recording that the models declined the %s: %w", question, err)
		}
		applied = tag.RowsAffected() > 0
		return nil
	})
	return applied, err
}
