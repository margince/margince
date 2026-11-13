// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A page of email rows names whom each message was with, as the message's own
// viewer does. That is the recipients of a sent message and the sender of a
// received one. The Worklist's follow-up rows read this page.
func TestAPageOfEmailRowsNamesTheFarSideAsTheViewerDoes(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	e.exec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, 'buyer@far.test', 'seed', 'system')`, contact)
	sent := seedPartiedEmail(t, e, "outbound", []partyRow{
		{role: "from", user: &e.rep},
		{role: "to", contact: &contact, address: "buyer@far.test"},
		{role: "to", address: "second@far.test"},
	})
	received := seedPartiedEmail(t, e, "inbound", []partyRow{
		{role: "from", contact: &contact, address: "buyer@far.test"},
		{role: "to", user: &e.rep},
	})

	page, err := storeKnowing(e).EmailSummariesByID(e.as(), []ids.UUID{sent, received})
	if err != nil {
		t.Fatalf("reading the page: %v", err)
	}
	for _, tc := range []struct {
		name, want string
		id         ids.UUID
	}{
		{"sent", "Buyer Contact +1", sent},
		{"received", "Buyer Contact", received},
	} {
		row, ok := page[tc.id]
		if !ok {
			t.Fatalf("%s: the page has no row for the message", tc.name)
		}
		if row.Counterparty == nil || *row.Counterparty != tc.want {
			t.Errorf("%s: the row names %v, want %q", tc.name, ptrText(row.Counterparty), tc.want)
		}
		if row.CounterpartyContactId == nil || ids.UUID(*row.CounterpartyContactId) != contact {
			t.Errorf("%s: the row names contact %v, want %s", tc.name, row.CounterpartyContactId, contact)
		}
		viewer, err := storeKnowing(e).GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](tc.id), nil)
		if err != nil {
			t.Fatalf("%s: reading the message: %v", tc.name, err)
		}
		if !sameCounterparty(row, viewer.Summary) {
			t.Errorf("%s: the page names %v (%v), the viewer %v (%v)", tc.name,
				ptrText(row.Counterparty), row.CounterpartyContactId,
				ptrText(viewer.Summary.Counterparty), viewer.Summary.CounterpartyContactId)
		}
	}
}

type partyRow struct {
	role, address string
	contact, user *ids.UUID
}

func seedPartiedEmail(t *testing.T, e *loadEnv, direction string, parties []partyRow) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, 'Great to meet you', now(), 'seed', 'system')`, activity, direction)
	for _, row := range parties {
		e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id, user_id, address)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))`,
			ids.NewV7(), activity, row.role, row.contact, row.user, row.address)
	}
	return activity
}

func sameCounterparty(a, b crmcontracts.EmailSummary) bool {
	if (a.Counterparty == nil) != (b.Counterparty == nil) ||
		(a.CounterpartyContactId == nil) != (b.CounterpartyContactId == nil) {
		return false
	}
	return (a.Counterparty == nil || *a.Counterparty == *b.Counterparty) &&
		(a.CounterpartyContactId == nil || *a.CounterpartyContactId == *b.CounterpartyContactId)
}

func ptrText(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
