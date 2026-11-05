// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Google scopes a mailbox grant carries to read and to send.
const (
	GmailReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"
	GmailSendScope     = "https://www.googleapis.com/auth/gmail.send"
)

// DiscardingMailer is the relay boundary for a suite that reads the link from
// the vault rather than from a message.
//
// It is still wired: a controller delivery resolves its transport before any
// gate runs. With no relay the message would park as "no mail relay configured",
// and the suite would test the unconfigured path instead.
type DiscardingMailer struct{}

func (DiscardingMailer) Send(context.Context, string, string, string) error { return nil }

// AskedAboutTheDeal plants the contact's own enquiry about a deal. It is mail
// from them that a connector captured from an authenticated sender. It is filed
// on the deal. An open deal they are staked on carries a follow-up only with
// such a request behind it. A deal we created, or a hand-logged activity, does
// not.
func AskedAboutTheDeal(t *testing.T, e *AppEnv, contactID, address, dealID string) {
	t.Helper()
	ctx := context.Background()
	enquiry := ids.NewV7()
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by, inbound_sender_authenticated)
		VALUES ($1, 'email', 'inbound', $2, $3, 'gmail', 'connector:gmail', true)`,
		enquiry, "enquiry-"+enquiry.String(), clocktest.Now(t).Add(-24*time.Hour)); err != nil {
		t.Fatalf("planting the enquiry: %v", err)
	}
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, contact_id, address, address_domain_folded, role)
		VALUES ($1, $2, $3, $4, 'from')`, enquiry, contactID, address, owndomain.StoredOfAddress(address)); err != nil {
		t.Fatalf("planting the enquiry's sender: %v", err)
	}
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`,
		enquiry, dealID); err != nil {
		t.Fatalf("filing the enquiry on the deal: %v", err)
	}
}
