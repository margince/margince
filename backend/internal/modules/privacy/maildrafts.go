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
func redactUnsentMessages(ctx context.Context, tx pgx.Tx, reason string, subject ids.ContactID, leads []ids.UUID, emails []string) error {
	if err := redactScheduledSends(ctx, tx, reason, emails); err != nil {
		return err
	}
	return purgeMailDrafts(ctx, tx, subject, leads, emails)
}

// subjectDraftMatch is which drafts are the subject's: opened on their contact
// or one of their lead twins, or addressed to one of their addresses on any
// line. The erasure and the export share it, so a subject is never shown a
// draft the erasure would leave behind, nor the reverse. Addresses compare
// trimmed and lowercased, the way the send path compares them.
//
// $1 contact, $2 lead ids, $3 the subject's addresses from loweredAddresses.
const subjectDraftMatch = `
	   (d.anchor_type = 'contact' AND d.anchor_id = $1)
	OR (d.anchor_type = 'lead'    AND d.anchor_id = ANY($2::uuid[]))
	OR EXISTS (
	     SELECT 1 FROM unnest(d.to_addresses || d.cc_addresses || d.bcc_addresses) AS addressed(address)
	      WHERE lower(btrim(addressed.address)) = ANY($3::text[]))`

// purgeMailDrafts deletes every draft subjectDraftMatch names.
//
// DELETED rather than emptied, because a draft is nobody's record: it is the
// composer's state, readable by one rep, and a blank one left behind would
// only reopen as an empty message. No tombstone either — the audit images of a
// draft carry its anchor and version and never its words, so there is nothing
// quoted for a tombstone to stop a reader at.
func purgeMailDrafts(ctx context.Context, tx pgx.Tx, subject ids.ContactID, leads []ids.UUID, emails []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM mail_draft d WHERE `+subjectDraftMatch,
		subject.UUID, leads, loweredAddresses(emails)); err != nil {
		return fmt.Errorf("purging the unsent drafts addressed to the subject: %w", err)
	}
	return nil
}

// purgeContactMailDrafts is purgeMailDrafts for the retention anonymise, which
// has not already gathered the subject's lead twins the way the erasure has.
func purgeContactMailDrafts(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
	_, leads, _, err := subjectReach(ctx, tx, subject)
	if err != nil {
		return err
	}
	return purgeMailDrafts(ctx, tx, subject, leads, emails)
}
