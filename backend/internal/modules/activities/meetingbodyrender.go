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
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// MeetingDescriptionRenderedFlag marks the audit row of a meeting body whose
// stored HTML description was rendered to text. A marked meeting is never
// rendered again, so a replay cannot render text that merely looks like HTML.
const MeetingDescriptionRenderedFlag = "description_rendered"

// MeetingBodyRender is a calendar meeting body rendered from its stored HTML
// description, and the row it was rendered from as it was read.
type MeetingBodyRender struct {
	Activity   ids.UUID
	Version    int64
	StoredBody string
	Body       string
}

// RenderMeetingBodyTx writes a calendar meeting body whose stored description
// was raw HTML. It writes nothing when the row changed since it was read, is
// held, or a member ever edited its body: that text is theirs. The meeting
// time stays. The language is detected again from the new body and subject,
// as capture detects it.
func RenderMeetingBodyTx(ctx context.Context, tx pgx.Tx, r MeetingBodyRender) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", r.Activity, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var subject, before *string
	if err := tx.QueryRow(ctx, `SELECT subject, language FROM activity WHERE id = $1`, r.Activity).
		Scan(&subject, &before); err != nil {
		return false, fmt.Errorf("activities: reading a meeting to render its description: %w", err)
	}
	var language *string
	if detected := string(textlang.DetectFirst(r.Body, deref(subject))); detected != "" {
		language = &detected
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity a SET body = $2, language = $3
		 WHERE a.id = $1 AND a.version = $4 AND a.body = $5 AND a.kind = 'meeting' AND a.restricted_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM audit_log edit
		         WHERE edit.entity_type = 'activity' AND edit.entity_id = a.id
		           AND (edit.actor_type = 'human' AND (edit.after ? 'body' OR edit.before ? 'body')
		             OR edit.after ? '`+MeetingDescriptionRenderedFlag+`'))`,
		r.Activity, r.Body, language, r.Version, r.StoredBody)
	if err != nil {
		return false, fmt.Errorf("activities: rendering a meeting description: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// The body stays out of the audit row, as at capture. The trail records
	// that the description was rendered and what that changed.
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", r.Activity,
		map[string]any{"language": before},
		map[string]any{"language": language, MeetingDescriptionRenderedFlag: true})
	if err != nil {
		return false, err
	}
	touched := true
	if err := storekit.EmitEvent(ctx, tx, auditID, r.Activity, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched},
	}); err != nil {
		return false, err
	}
	return true, nil
}
