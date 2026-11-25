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
// source_revision holds the provider's edit time of the stored text. An edit
// is written only when it is newer. A redelivered or reordered report writes
// no audit row and emits no event. A message the connector did not file, an
// erased one, and one whose text a member rewrote are left alone.
//
// The language, label, model verdicts and declines were read from the old text.
// They are cleared, as when a mail body is restored. The activity.updated
// event with the body flag drives the search graph and the assurance inputs.
//
// found says a row holds the key. applied says the text was written.
func ApplyProviderEditTx(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, connectorID, body string, revision time.Time,
) (found, applied bool, err error) {
	// A connector's grant to file a message is its grant to restate it.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return false, false, err
	}
	var id ids.UUID
	var archived bool
	var stored *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, archived_at IS NOT NULL, source_revision FROM activity
		 WHERE source_system = $1 AND source_id = $2
		   AND (captured_by = $3 OR starts_with(captured_by, $3 || ':'))
		   FOR UPDATE`,
		key.SourceSystem, key.SourceID, connectorID).Scan(&id, &archived, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("activities: finding the message a provider edited: %w", err)
	}
	if archived || (stored != nil && !revision.After(*stored)) {
		return true, false, nil
	}
	if err := auth.EnsureActivityVisible(ctx, tx, id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return true, false, nil
		}
		return false, false, err
	}
	written, replyCleared, err := writeProviderEdit(ctx, tx, id, body, revision)
	if err != nil || !written {
		return true, false, err
	}
	if err := recordReplyVerdictCleared(ctx, tx, id, replyCleared); err != nil {
		return true, false, err
	}
	// The text stays out of the audit image, as at capture.
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id,
		map[string]any{"source_revision": stored},
		map[string]any{"source_revision": revision.UTC(), "body_edited_at_provider": true})
	if err != nil {
		return true, false, err
	}
	touched := true
	if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched},
	}); err != nil {
		return true, false, err
	}
	return true, true, nil
}

// writeProviderEdit stores the text and the revision under the row lock, and
// clears what the old text produced. It reports whether a reply verdict was
// cleared, which the verdict history must record.
func writeProviderEdit(ctx context.Context, tx pgx.Tx, id ids.UUID, body string, revision time.Time) (written, replyCleared bool, err error) {
	err = tx.QueryRow(ctx, `
		WITH old AS (SELECT reply_verdict FROM activity WHERE id = $1)
		UPDATE activity a SET body = $2, source_revision = $3, language = NULL,
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
		id, body, revision).Scan(&replyCleared)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("activities: writing a provider's edit of a message: %w", err)
	}
	return true, replyCleared, nil
}
