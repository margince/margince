// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// askedAboutTheDeal plants the contact's own enquiry about a deal: mail from
// them that a connector captured from an authenticated sender, filed on the
// deal. An open deal they are staked on carries a follow-up only with such a
// request behind it; a deal we created, or a hand-logged activity, does not.
func askedAboutTheDeal(t *testing.T, e *apptest.AppEnv, contactID, address, dealID string) {
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
		INSERT INTO activity_participant (activity_id, contact_id, address, role)
		VALUES ($1, $2, $3, 'from')`, enquiry, contactID, address); err != nil {
		t.Fatalf("planting the enquiry's sender: %v", err)
	}
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`,
		enquiry, dealID); err != nil {
		t.Fatalf("filing the enquiry on the deal: %v", err)
	}
}
