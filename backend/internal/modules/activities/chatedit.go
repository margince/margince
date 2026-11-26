// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ApplyProviderEditTx writes the text a chat provider says a message now has,
// over the message a connector filed under this natural key.
//
// `activity` is this module's table, so capture calls through a seam compose
// injects.
//
// source_revision and source_revision_seq hold the provider's edit time and
// report sequence of the stored text. An edit is written only when it is later
// in that order. A redelivered or reordered report writes no audit row and
// emits no event. A written edit links the activity to its own raw row and
// answers the row it replaced, which the caller deletes. A message the connector did not file, an
// erased one, and one whose text a member rewrote are left alone.
//
// The language, label, model verdicts and declines were read from the old text.
// They are cleared, as when a mail body is restored. The activity.updated
// event with the body flag drives the search graph and the assurance inputs.
//
// found says a row holds the key. applied says the text was written.
func ApplyProviderEditTx(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, connectorID string, change connector.ChatChange,
) (found, applied bool, superseded ids.UUID, err error) {
	// A connector's grant to file a message is its grant to restate it.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return false, false, ids.Nil, err
	}
	revision := change.Revision.Truncate(time.Microsecond)
	target, found, err := lockEditTarget(ctx, tx, key, connectorID)
	if err != nil || !found {
		return false, false, ids.Nil, err
	}
	if target.archived || !target.acceptsEditAt(revision, change.RevisionSeq) {
		return true, false, ids.Nil, nil
	}
	if err := auth.EnsureActivityVisible(ctx, tx, target.id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return true, false, ids.Nil, nil
		}
		return false, false, ids.Nil, err
	}
	written, replyCleared, err := writeProviderEdit(ctx, tx, target.id, change, revision)
	if err != nil || !written {
		return true, false, ids.Nil, err
	}
	if err := recordReplyVerdictCleared(ctx, tx, target.id, replyCleared); err != nil {
		return true, false, ids.Nil, err
	}
	if err := emitProviderEdit(ctx, tx, target, revision); err != nil {
		return true, false, ids.Nil, err
	}
	if target.raw != nil && *target.raw != change.StoredOriginal {
		superseded = *target.raw
	}
	return true, true, superseded, nil
}

// editTarget is the message row a provider's edit is about, as locked.
type editTarget struct {
	id       ids.UUID
	archived bool
	stored   *time.Time
	seq      int64
	raw      *ids.UUID
}

// acceptsEditAt reports whether an edit is later than the stored text in the
// provider's order: its time, then its report sequence.
func (t editTarget) acceptsEditAt(revision time.Time, seq int64) bool {
	return t.stored == nil || revision.After(*t.stored) || (revision.Equal(*t.stored) && seq > t.seq)
}

// lockEditTarget locks the row the connector filed under the key.
func lockEditTarget(ctx context.Context, tx pgx.Tx, key connector.NaturalKey, connectorID string) (editTarget, bool, error) {
	var t editTarget
	err := tx.QueryRow(ctx, `
		SELECT id, archived_at IS NOT NULL, source_revision, source_revision_seq, raw_capture_id FROM activity
		 WHERE source_system = $1 AND source_id = $2
		   AND (captured_by = $3 OR starts_with(captured_by, $3 || ':'))
		   FOR UPDATE`,
		key.SourceSystem, key.SourceID, connectorID).Scan(&t.id, &t.archived, &t.stored, &t.seq, &t.raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return editTarget{}, false, nil
	}
	if err != nil {
		return editTarget{}, false, fmt.Errorf("activities: finding the message a provider edited: %w", err)
	}
	return t, true, nil
}

// emitProviderEdit audits the edit and announces the body change. The text
// stays out of the audit image, as at capture.
func emitProviderEdit(ctx context.Context, tx pgx.Tx, t editTarget, revision time.Time) error {
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", t.id,
		map[string]any{"source_revision": t.stored},
		map[string]any{"source_revision": revision.UTC(), "body_edited_at_provider": true})
	if err != nil {
		return err
	}
	touched := true
	return storekit.EmitEvent(ctx, tx, auditID, t.id, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched},
	})
}

// writeProviderEdit stores the text and the revision under the row lock, and
// clears what the old text produced. It reports whether a reply verdict was
// cleared, which the verdict history must record.
func writeProviderEdit(ctx context.Context, tx pgx.Tx, id ids.UUID, change connector.ChatChange, revision time.Time) (written, replyCleared bool, err error) {
	err = tx.QueryRow(ctx, `
		WITH old AS (SELECT reply_verdict FROM activity WHERE id = $1)
		UPDATE activity a SET body = $2, source_revision = $3, source_revision_seq = $4,
		       raw_capture_id = COALESCE($5, a.raw_capture_id), language = NULL,
		       capture_label = NULL, capture_labeled_at = NULL,
		       `+declineStampsClearedWhen("true")+`,
		       reply_verdict = CASE WHEN `+humanJudgedReplySQL+` THEN a.reply_verdict END,
		       reply_verdict_at = CASE WHEN `+humanJudgedReplySQL+` THEN a.reply_verdict_at END,
		       reply_verdict_by = CASE WHEN `+humanJudgedReplySQL+` THEN a.reply_verdict_by END,
		       owed_verdict_ruleset = CASE WHEN `+humanReviewedOwedSQL("a")+` THEN a.owed_verdict_ruleset END
		  FROM old
		 WHERE a.id = $1 AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND `+mailTextIsTheirsSQL(false)+`
		RETURNING old.reply_verdict IS NOT NULL AND a.reply_verdict IS NULL`,
		id, change.Body, revision, change.RevisionSeq, nullableRaw(change.StoredOriginal)).Scan(&replyCleared)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("activities: writing a provider's edit of a message: %w", err)
	}
	return true, replyCleared, nil
}

// nullableRaw is the raw row to link, or nil when the edit names none.
func nullableRaw(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}
