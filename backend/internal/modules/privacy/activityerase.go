// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
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
	return s.eraser.eraseActivityContent(ctx, tx, id, theClockRanOut)
}

// eraseActivityContent erases an activity's content, for the nightly erase
// action and a controller's archive deletion alike. The caller has already
// decided that nothing keeps the record.
func (e *Eraser) eraseActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID, act erasureAct) error {
	err := clearActivityContent(ctx, tx, id)
	if err == nil {
		// The same purge the restriction lift and the release run, so none drift.
		err = e.purgeContentDerivedFrom(ctx, tx, id, act)
	}
	if err == nil && e.invalidateEdges != nil {
		// The aggregates counted this interaction. Without an invalidator, the
		// retention.applied consumer re-folds them.
		err = e.invalidateEdges(ctx, tx, []ids.UUID{id})
	}
	return err
}

func clearActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE activity SET body = NULL, body_own_end = NULL, owed_ask = NULL, system_text = @system_text, raw = NULL, agenda_fingerprint = NULL, calendar_event = NULL, subject = @subject,
 internal_mail_authorization = `+erasedInternalMailAuthorizationSQL+`,
 language = NULL, source_author_name = NULL, reply_verdict = NULL, reply_verdict_at = NULL,
 reply_verdict_by = NULL, archived_at = coalesce(archived_at, now()) WHERE id = @id`, pgx.StrictNamedArgs{"subject": erasedActivitySubject, "id": id, "system_text": erasedActivityText()})
	if err != nil {
		return err
	}
	return clearActivityCustomValues(ctx, tx, []ids.UUID{id})
}

// erasedActivityText marks an erased activity; only erasure writes it.
func erasedActivityText() *systemtext.Message {
	return systemtext.BoundLiteral("core.erased", erasedActivitySubject)
}

// A tombstone keeps replay restricted after source identifiers and folder names are erased.
const erasedInternalMailAuthorizationSQL = `CASE WHEN internal_mail_authorization IS NULL THEN NULL
 ELSE '{"connection_id":null,"owner_user_id":null,"provider":null,"generation":null,"enrolled_at":null,"enrolled_by":null,"policy_since":null}'::jsonb END`
