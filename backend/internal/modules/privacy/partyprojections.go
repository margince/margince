// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *Eraser) purgePartyProjections(ctx context.Context, tx pgx.Tx, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	if e.purgeRawCaptures != nil {
		return e.purgeRawCaptures.PurgePartySourcesForEmailsTx(ctx, tx, emails)
	}
	var held bool
	if err := tx.QueryRow(ctx, `WITH erased AS (
 SELECT ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address) AS addresses)
 SELECT EXISTS(SELECT 1 FROM capture_party_source p
   WHERE p.recipients && erased.addresses OR p.policy_addresses && erased.addresses)
 OR EXISTS(SELECT 1 FROM capture_thread_verdict v, unnest(v.seen_addresses) seen
   WHERE lower(seen)=ANY(erased.addresses))
 OR EXISTS(SELECT 1 FROM capture_test_mailbox_sent,
   jsonb_array_elements_text(to_addresses || cc_addresses) address
   WHERE lower(btrim(address))=ANY(erased.addresses))
 FROM erased`, pgx.NamedArgs{"emails": emails}).Scan(&held); err != nil {
		return err
	}
	if held {
		return ErrRetentionSeamMissing
	}
	return nil
}

// purgeTextCopies empties what other modules copied from the redacted
// activities' text. An eraser built without the purger erases no mail
// content in production, so it has no copy to reach.
func (e *Eraser) purgeTextCopies(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	if e.purgeRawCaptures == nil || len(activities) == 0 {
		return nil
	}
	return e.purgeRawCaptures.PurgeTextCopiesTx(ctx, tx, activities)
}

// purgeTextNaming empties what other modules copied that names the subject,
// such as a voice sample greeting them. A replay reads the tombstone name, which
// names nobody. Every eraser compose builds for a request wires the purger.
func (e *Eraser) purgeTextNaming(ctx context.Context, tx pgx.Tx, keys subjectKeys) error {
	if e.purgeRawCaptures == nil {
		return nil
	}
	name := keys.displayName
	if name == erasedName {
		name = ""
	}
	return e.purgeRawCaptures.PurgeTextNamingTx(ctx, tx, name, keys.emails)
}
