// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func purgePartySourcesTx(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE capture_party_source SET raw_capture_id=NULL, sent_by_owner=false, containers='{}', containers_known=false, recipients='{}', policy_addresses='{}', admission_blocked=true, admission_pending=false WHERE activity_id=ANY(@activities::uuid[])`, pgx.NamedArgs{"activities": activities})
	return err
}

// PurgePartySourcesForEmailsTx preserves other recipients while erasing the
// subject proof. The thread ledger's seen addresses go too: a verdict outlives
// its message and the held list searches them. A later message from that
// address is then judged again, because an opening answer needs a sender it saw.
func (*PendingStore) PurgePartySourcesForEmailsTx(ctx context.Context, tx pgx.Tx, emails []string) error {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM capture_test_mailbox_sent
	 WHERE EXISTS (SELECT 1 FROM jsonb_array_elements_text(to_addresses || cc_addresses) address
	 WHERE lower(btrim(address))=ANY(ARRAY(SELECT lower(btrim(email)) FROM unnest(@emails::text[]) email)))`,
		pgx.StrictNamedArgs{"emails": emails}); err != nil {
		return err
	}
	// Erasure removes the subject's projection while preserving other recipients and privacy holds.
	_, err := tx.Exec(ctx, `WITH erased AS (SELECT ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address) AS addresses)
 UPDATE capture_party_source ps SET
 recipients=ARRAY(SELECT address FROM unnest(ps.recipients) address WHERE NOT(lower(address)=ANY(erased.addresses))),
 policy_addresses=ARRAY(SELECT address FROM unnest(ps.policy_addresses) address WHERE NOT(lower(address)=ANY(erased.addresses)))
 FROM erased WHERE ps.recipients && erased.addresses OR ps.policy_addresses && erased.addresses`, pgx.NamedArgs{"emails": emails})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH erased AS (SELECT ARRAY(SELECT lower(btrim(address)) FROM unnest(@emails::text[]) address) AS addresses)
 UPDATE capture_thread_verdict v SET
 disposition_reason=NULL,
 seen_addresses=ARRAY(SELECT address FROM unnest(v.seen_addresses) address WHERE NOT(lower(address)=ANY(erased.addresses)))
 FROM erased WHERE EXISTS (SELECT 1 FROM unnest(v.seen_addresses) address WHERE lower(address)=ANY(erased.addresses))`, pgx.NamedArgs{"emails": emails})
	return err
}
