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

// rulesetColumn and captureLabelColumn name the columns a restoration resets in
// the audit images.
const (
	rulesetColumn      = "owed_verdict_ruleset"
	captureLabelColumn = "capture_label"
)

// MailBodyRestoration is a mail body re-derived from its stored original, and
// the row it was derived for as it was read.
type MailBodyRestoration struct {
	Activity     ids.UUID
	Version      int64
	StoredBody   string
	Body         string
	Completeness string
	// Subject, when set, replaces the stored subject StoredSubject, which must
	// still be on the row. Nil leaves the subject alone.
	Subject       *string
	StoredSubject string
}

// RestoreMailBodyTx writes a mail body re-derived from its stored original.
// It writes nothing when the row changed since it was read, or when a member
// ever edited its body: that text is theirs. The occurrence time stays, so the
// mail keeps its age. The language, the attention label and an automatic
// request verdict were read from the old text. The first two are cleared, the
// verdict and its decline stamp are reset for re-judgement, and the thread is answered as stale.
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
		label        *string
	}
	var threadKey *string
	err := tx.QueryRow(ctx, `
		WITH old AS (SELECT body_completeness, owed_verdict_ruleset, capture_label FROM activity WHERE id = $1 AND restricted_at IS NULL)
		UPDATE activity a SET body = $2, body_completeness = $3, language = NULL,
		       subject = coalesce($6::text, a.subject),
		       capture_label = NULL, capture_labeled_at = NULL,
		       capture_label_declined_at = NULL, capture_label_declined_ruleset = NULL,
		       owed_verdict_declined_at = NULL, owed_verdict_declined_ruleset = NULL,
		       owed_verdict_ruleset = CASE WHEN `+humanReviewedOwedSQL("a")+` THEN a.owed_verdict_ruleset END
		  FROM old
		 WHERE a.id = $1 AND a.version = $4 AND a.body = $5
		   AND ($6::text IS NULL OR a.subject = $7)
		   AND NOT EXISTS (SELECT 1 FROM audit_log edit
		         WHERE edit.entity_type = 'activity' AND edit.entity_id = a.id AND edit.actor_type = 'human'
		           AND (edit.after ? 'body' OR edit.before ? 'body'
		                OR ($6::text IS NOT NULL AND (edit.after ? 'subject' OR edit.before ? 'subject'))))
		RETURNING old.body_completeness, old.owed_verdict_ruleset, old.capture_label,
		          a.body_completeness, a.owed_verdict_ruleset, a.thread_key`,
		r.Activity, r.Body, r.Completeness, r.Version, r.StoredBody, r.Subject, r.StoredSubject).
		Scan(&before.completeness, &before.ruleset, &before.label, &after.completeness, &after.ruleset, &threadKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("activities: restoring a mail body from its original: %w", err)
	}
	// The body itself stays out of the audit row, as at capture; the trail
	// records that it was restored and what that changed.
	auditAfter := map[string]any{"body_completeness": after.completeness, rulesetColumn: after.ruleset, captureLabelColumn: nil, "body_restored": true}
	if r.Subject != nil {
		auditAfter["subject_restored"] = true
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", r.Activity,
		map[string]any{"body_completeness": before.completeness, rulesetColumn: before.ruleset, captureLabelColumn: before.label},
		auditAfter)
	if err != nil {
		return false, nil, err
	}
	touched := true
	if err := storekit.EmitEvent(ctx, tx, auditID, r.Activity, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched, Subject: r.Subject},
	}); err != nil {
		return false, nil, err
	}
	return true, threadKey, nil
}
