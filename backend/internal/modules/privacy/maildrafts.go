// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The messages a rep started and never sent. A saved draft holds the subject's
// address and whatever was written to them, before any activity or scheduled
// send exists, so nothing else in the cascade reaches it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactUnsentMessages reaches both kinds of message that exist before any
// activity does: one scheduled for later, and one still in a rep's composer.
func redactUnsentMessages(ctx context.Context, tx pgx.Tx, reason string, subject ids.ContactID, emails []string) error {
	if err := redactScheduledSends(ctx, tx, reason, emails); err != nil {
		return err
	}
	return purgeMailDrafts(ctx, tx, subject, emails)
}

// purgeMailDrafts deletes every draft addressed to one of the subject's
// addresses or opened on the subject's own record.
//
// DELETED rather than emptied, because a draft is nobody's record: it is the
// composer's state, readable by one rep, and a blank one left behind would
// only reopen as an empty message. No tombstone either — the audit images of a
// draft carry its anchor and version and never its words, so there is nothing
// quoted for a tombstone to stop a reader at.
//
// Case-insensitive for the reason redactScheduledSends is: a draft keeps the
// address as the rep typed it.
func purgeMailDrafts(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		DELETE FROM mail_draft
		 WHERE (anchor_type = 'contact' AND anchor_id = $%d)
		    OR EXISTS (
		         SELECT 1
		           FROM unnest(to_addresses || cc_addresses || bcc_addresses) AS addressed(address)
		          WHERE lower(addressed.address) = ANY($%d))`,
		arg(subject.UUID), arg(loweredAddresses(emails))), args...); err != nil {
		return fmt.Errorf("purging the unsent drafts addressed to the subject: %w", err)
	}
	return nil
}
