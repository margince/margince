// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of one mail's words. A job tick or a bus delivery has no
// caller, so no grant narrows it. The read answers only mail the whole
// workspace may read, as the mail-by-deal read does, and answers anything else
// as a mail that does not exist, so an id cannot be used to probe.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) MailContent() extension.MailContents { return systemMailContent{rt: r} }

type systemMailContent struct{ rt *callRuntime }

const mailContentQuery = `
	SELECT a.id, a.subject, a.body, a.audience, a.capture_label, a.bulk_mail_attested, a.source_id, a.occurred_at, a.redacted_fields
	  FROM activity a
	 WHERE a.id = $1 AND a.kind = 'email' AND a.audience = 'workspace' AND a.archived_at IS NULL AND %s`

func (m systemMailContent) Get(ctx context.Context, mailID string) (extension.MailContent, error) {
	ctx, err := m.rt.systemRead(ctx, extension.SystemReadMailContent)
	if err != nil {
		return extension.MailContent{}, err
	}
	id, err := unitID(mailID, "mail")
	if err != nil {
		return extension.MailContent{}, err
	}
	if err := m.rt.traceSystemRead(ctx, extension.SystemReadMailContent, "activity", mailID); err != nil {
		return extension.MailContent{}, err
	}
	out := extension.MailContent{ID: id.String(), ContentState: extension.MailContentAvailable}
	err = database.WithWorkspaceTx(ctx, m.rt.deps.pool, func(tx pgx.Tx) error {
		var (
			subject, body, label, source *string
			redacted                     []string
			at                           time.Time
		)
		err := tx.QueryRow(ctx, fmt.Sprintf(mailContentQuery, auth.ActivityAvailableClause("a")), id).Scan(&id, &subject, &body, &out.Audience, &label, &out.BulkMail, &source, &at, &redacted)
		if err != nil {
			return err
		}
		out.OccurredAt = at
		if label != nil {
			out.CaptureLabel = *label
		}
		if source != nil {
			out.SourceID = *source
		}
		if slices.Contains(redacted, "subject") || slices.Contains(redacted, "body") {
			out.ContentState = extension.MailContentWithheld
			return nil
		}
		if subject != nil {
			out.Subject = *subject
		}
		if body != nil {
			out.Body = *body
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return extension.MailContent{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.MailContent{}, m.rt.systemReadFault(ctx, err)
	}
	return out, nil
}
