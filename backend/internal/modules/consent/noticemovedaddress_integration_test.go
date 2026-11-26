// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A bounced notice to the new holder of a moved address is not settled by mail
// sent from it before. That holds when the sender row names the former holder,
// and when it carries only the address.
func TestAFailedNoticeIsNotSettledByTheFormerHoldersMail(t *testing.T) {
	for _, attributed := range []bool{true, false} {
		failedNoticeAfterMovedMail(t, attributed)
	}
}

func failedNoticeAfterMovedMail(t *testing.T, attributed bool) {
	t.Helper()
	e := setupChannelConsent(t)
	former, delivery := ids.New[ids.ContactKind](), seedDelivery(t, e)
	var owed ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		WITH former AS (
		  INSERT INTO contact (id, full_name, source, captured_by) VALUES (@former, 'Former Holder', 'manual', 'human:seed')),
		addresses AS (
		  INSERT INTO contact_email (contact_id, email, source, captured_by, archived_at)
		  VALUES (@former, @address, 'manual', 'human:seed', now()), (@holder, @address, 'manual', 'human:seed', NULL)),
		mail AS (
		  INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by, bulk_mail_attested, inbound_sender_authenticated)
		  VALUES ('email', 'hello', 'inbound', now() - interval '1 year', 'gmail', 'connector:gmail', false, true) RETURNING id),
		sender AS (
		  INSERT INTO activity_participant (activity_id, role, contact_id, address) SELECT id, 'from', CASE WHEN @attributed THEN @former::uuid END, @address FROM mail),
		acquisition AS (
		  INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
		  VALUES (@holder, 'unknown_legacy', 'connector:gmail') RETURNING id)
		INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state, delivery_id)
		SELECT @holder, id, 'art14', now() + interval '30 days', 'queued', @delivery FROM acquisition
		RETURNING id`, pgx.NamedArgs{
		"former": former, "holder": e.contact, "address": "moved@notice.test", "delivery": delivery, "attributed": attributed,
	}).Scan(&owed); err != nil {
		t.Fatalf("seeding the moved address and the queued notice: %v", err)
	}

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := MarkNoticeDeliveryFailedTx(e.ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("recording the failed delivery: %v", err)
	}

	if state, _ := noticeCaseState(t, e, owed); state == string(NoticeExemptWithReason) {
		t.Errorf("attributed %v: the new holder's failed notice was settled by mail sent from the moved address", attributed)
	}
}
