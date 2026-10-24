// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rulesetColumn names the verdict's ruleset in the audit images.
const rulesetColumn = "owed_verdict_ruleset"

// MailBodyRestoration is a mail body re-derived from its stored original, and
// the row it was derived for as it was read.
type MailBodyRestoration struct {
	Activity     ids.UUID
	Version      int64
	StoredBody   string
	Body         string
	Completeness string
}

// RestoreMailBodyTx writes a mail body re-derived from its stored original.
// It writes nothing when the row changed since it was read, or when a member
// ever edited its body: that text is theirs. The occurrence time stays, so the
// mail keeps its age. The language and an automatic request verdict were read
// from the old text. So the language is cleared, and the verdict is marked for
// the next re-judgement. It answers the thread whose reading is now stale.
func RestoreMailBodyTx(ctx context.Context, tx pgx.Tx, r MailBodyRestoration) (bool, *string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, nil, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", r.Activity, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}
	var before, after struct {
		completeness string
		ruleset      *string
	}
	var threadKey *string
	err := tx.QueryRow(ctx, `
		WITH old AS (SELECT body_completeness, owed_verdict_ruleset FROM activity WHERE id = $1 AND restricted_at IS NULL)
		UPDATE activity a SET body = $2, body_completeness = $3, language = NULL,
		       owed_verdict_ruleset = CASE WHEN `+humanReviewedOwedSQL("a")+` THEN a.owed_verdict_ruleset END
		  FROM old
		 WHERE a.id = $1 AND a.version = $4 AND a.body = $5
		   AND NOT EXISTS (SELECT 1 FROM audit_log edit
		         WHERE edit.entity_type = 'activity' AND edit.entity_id = a.id AND edit.actor_type = 'human'
		           AND (edit.after ? 'body' OR edit.before ? 'body'))
		RETURNING old.body_completeness, old.owed_verdict_ruleset, a.body_completeness, a.owed_verdict_ruleset, a.thread_key`,
		r.Activity, r.Body, r.Completeness, r.Version, r.StoredBody).
		Scan(&before.completeness, &before.ruleset, &after.completeness, &after.ruleset, &threadKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("activities: restoring a mail body from its original: %w", err)
	}
	// The body itself stays out of the audit row, as at capture; the trail
	// records that it was restored and what that changed.
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", r.Activity,
		map[string]any{"body_completeness": before.completeness, rulesetColumn: before.ruleset},
		map[string]any{"body_completeness": after.completeness, rulesetColumn: after.ruleset, "body_restored": true})
	if err != nil {
		return false, nil, err
	}
	touched := true
	if err := storekit.EmitEvent(ctx, tx, auditID, r.Activity, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched},
	}); err != nil {
		return false, nil, err
	}
	return true, threadKey, nil
}
