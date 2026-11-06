// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

//nolint:goconst // the named-argument keys spell this statement's own parameters
func discardHeldDraftTx(ctx context.Context, tx pgx.Tx, held claimedSend, origin SendOrigin, in SendEmailInput) error {
	if held.PendingDraftID != nil {
		return discardComposedDraftForAuthorTx(ctx, tx, *held.PendingDraftID, origin, held.ScheduledBy)
	}
	// Older held messages have no draft ID. Only an unchanged draft of this
	// sender's exact message may be discarded without that identity.
	types, anchors := origin.draftAnchors()
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM mail_draft
		 WHERE author_id = @author
		   AND (anchor_type, anchor_id) IN (SELECT * FROM unnest(@types::text[], @anchors::uuid[]))
		   AND subject = @subject AND body = @body
		   AND coalesce(html_body, '') = @html AND updated_at <= @held_at
		   AND to_addresses = @recipients::text[]
		   AND cc_addresses = @cc::text[] AND bcc_addresses = @bcc::text[]
		 ORDER BY id LIMIT 1 FOR UPDATE`, pgx.NamedArgs{
		"author": held.ScheduledBy, "types": types, "anchors": anchors,
		"subject": in.Subject, "body": in.Body, "recipients": addressLine(in.Recipients),
		"html": in.HTMLBody, "held_at": held.CreatedAt,
		"cc": addressLine(in.Cc), "bcc": addressLine(in.Bcc),
	}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("held send: finding the composed draft: %w", err)
	}
	return discardComposedDraftForAuthorTx(ctx, tx, id, origin, held.ScheduledBy)
}
