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
//
// Subject is the subject re-derived from the same original, or nil to leave the
// stored one. A subject only arrives from a parser that read the charset the
// message declared, so it is written under the same guards as the body.
type MailBodyRestoration struct {
	Activity     ids.UUID
	Version      int64
	StoredBody   string
	Body         string
	Completeness string
	Subject      *string
}

// restoredImage is the trail's after image. The text itself stays out of it, as
// at capture: the trail records which text was re-derived, not what it says.
func restoredImage(completeness string, ruleset *string, subjectRedecoded bool) map[string]any {
	image := map[string]any{
		"body_completeness": completeness, rulesetColumn: ruleset, "body_restored": true,
	}
	if subjectRedecoded {
		image["subject_redecoded"] = true
	}
	return image
}

// mailTextIsTheirsSQL declines a row whose text a member ever edited. The keys
// are the columns this write touches: a member who rewrote the subject owns it
// just as one who rewrote the body does.
func mailTextIsTheirsSQL(withSubject bool) string {
	keys := "edit.after ? 'body' OR edit.before ? 'body'"
	if withSubject {
		keys += " OR edit.after ? 'subject' OR edit.before ? 'subject'"
	}
	return `NOT EXISTS (SELECT 1 FROM audit_log edit
	         WHERE edit.entity_type = 'activity' AND edit.entity_id = a.id AND edit.actor_type = 'human'
	           AND (` + keys + `))`
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
		       subject = COALESCE($6, a.subject),
		       owed_verdict_ruleset = CASE WHEN `+humanReviewedOwedSQL("a")+` THEN a.owed_verdict_ruleset END
		  FROM old
		 WHERE a.id = $1 AND a.version = $4 AND a.body = $5
		   AND `+mailTextIsTheirsSQL(r.Subject != nil)+`
		RETURNING old.body_completeness, old.owed_verdict_ruleset, a.body_completeness, a.owed_verdict_ruleset, a.thread_key`,
		r.Activity, r.Body, r.Completeness, r.Version, r.StoredBody, r.Subject).
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
		restoredImage(after.completeness, after.ruleset, r.Subject != nil))
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
