// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of one mail's words. It runs as the unit's service member
// under the activity content gate the app applies: audience, retention hold,
// participants and the row scope. A restriction of processing (Art. 18) is
// added here, because that gate withholds it from an agent seat only. Anything
// else answers as a mail that does not exist, so an id cannot be used to probe.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/systemmail"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) MailContent() extension.MailContents { return systemMailContent{rt: r} }

type systemMailContent struct{ rt *callRuntime }

const mailContentQuery = `
	SELECT a.subject, a.body, a.audience, a.capture_label, a.bulk_mail_attested, a.source_id, a.occurred_at, a.redacted_fields
	  FROM activity a
	 WHERE a.id = $1 AND %s`

func (m systemMailContent) Get(ctx context.Context, actAs extension.UserID, mailID string) (extension.MailContent, error) {
	seatCtx, seat, err := m.rt.seatRead(ctx, extension.SystemReadMailContent, actAs)
	if err != nil {
		return extension.MailContent{}, err
	}
	id, err := unitID(mailID, "mail")
	if err != nil {
		return extension.MailContent{}, err
	}
	if err := seat.trace("activity", mailID, nil); err != nil {
		return extension.MailContent{}, err
	}
	out := extension.MailContent{ID: id.String(), ContentState: extension.MailContentAvailable}
	err = database.WithWorkspaceTx(seatCtx, m.rt.deps.pool, func(tx pgx.Tx) error {
		if err := auth.Require(seatCtx, "activity", principal.ActionRead); err != nil {
			return err
		}
		if err := auth.EnsureActivityContentVisibleLive(seatCtx, tx, id); err != nil {
			return err
		}
		var (
			subject, body, label, source *string
			redacted                     []string
			at                           time.Time
		)
		query := fmt.Sprintf(mailContentQuery, systemmail.ReadableMail("a"))
		err := tx.QueryRow(seatCtx, query, id).Scan(&subject, &body, &out.Audience, &label, &out.BulkMail, &source, &at, &redacted)
		if err != nil {
			return err
		}
		out.OccurredAt = at
		out.CaptureLabel, out.SourceID = derefString(label), derefString(source)
		if slices.Contains(redacted, "subject") || slices.Contains(redacted, "body") {
			out.ContentState = extension.MailContentWithheld
			return nil
		}
		out.Subject = derefString(subject)
		out.Body = derefString(body)
		if cut := textcut.Runes(out.Body, extension.MaxMailBody); cut != out.Body {
			out.Body, out.BodyTruncated = cut, true
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, apperrors.ErrNotFound) {
		return extension.MailContent{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.MailContent{}, seat.seatFault(err)
	}
	return out, nil
}
