// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The number a lead carries, and where it goes when the lead is promoted.
//
// A lead holds one number as a column of its own, the way it holds one email.
// It is a single enquiry until it is promoted. A contact's numbers are a list,
// so promotion moves the lead's one number into that list.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadPhone is the number the lead row carries, or empty.
func (e *promoteConsentEnv) leadPhone(t *testing.T, lead ids.LeadID) string {
	t.Helper()
	var phone *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT phone FROM lead WHERE id = $1`, lead).Scan(&phone); err != nil {
		t.Fatalf("reading lead %s: %v", lead, err)
	}
	if phone == nil {
		return ""
	}
	return *phone
}

// contactNumbers reads a contact's numbers back, in position order.
func (e *promoteConsentEnv) contactNumbers(t *testing.T, contact ids.ContactID) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT phone FROM contact_phone WHERE contact_id = $1 ORDER BY position`, contact)
	if err != nil {
		t.Fatalf("reading contact phones: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var phone string
		if err := rows.Scan(&phone); err != nil {
			t.Fatalf("scanning contact phone: %v", err)
		}
		out = append(out, phone)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading contact phones: %v", err)
	}
	return out
}

// seedLeadWithPhone is seedLead plus a number, written through the store so the
// normalisation under test is the one production applies.
func (e *promoteConsentEnv) seedLeadWithPhone(t *testing.T, email, phone string) ids.LeadID {
	t.Helper()
	lead := e.seedLead(t, email)
	if _, err := e.store.UpdateLead(e.ctx, lead, UpdateLeadInput{Phone: &phone}); err != nil {
		t.Fatalf("setting the lead's number: %v", err)
	}
	return lead
}

func TestALeadsNumberIsStoredInE164(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLeadWithPhone(t, "dialled@example.test", "+49 30 1234567")

	if got := e.leadPhone(t, lead); got != "+49301234567" {
		t.Errorf("lead phone = %q, want +49301234567: the spaces a human types are not part of the number", got)
	}
}

func TestANumberTypedWithoutACountryCodeIsRefused(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "local@example.test")

	local := "030 1234567"
	if _, err := e.store.UpdateLead(e.ctx, lead, UpdateLeadInput{Phone: &local}); err == nil {
		t.Fatal("a number with no country code was accepted: it is unreachable, and the phone dedupe needs E.164")
	}
	if got := e.leadPhone(t, lead); got != "" {
		t.Errorf("lead phone = %q after a refused write, want nothing", got)
	}
}

func TestAPromotedLeadsNumberReachesTheContactItBecomes(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLeadWithPhone(t, "fresh.number@example.test", "+493011112222")

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged {
		t.Fatalf("promote: err=%v merged=%t, want a created contact", err, merged)
	}

	numbers := e.contactNumbers(t, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
	if len(numbers) != 1 || numbers[0] != "+493011112222" {
		t.Errorf("contact numbers = %v, want the one the lead carried", numbers)
	}
}

func TestAPromotedLeadsNumberReachesTheContactItMergesInto(t *testing.T) {
	e := setupPromoteConsent(t)
	const shared = "known.number@example.test"
	existing, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Known Already",
		Emails:   []ContactEmailInput{{Email: shared, EmailType: emailTypeWork, IsPrimary: true, Position: 1}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("seeding the contact the lead will merge into: %v", err)
	}
	lead := e.seedLeadWithPhone(t, shared, "+493033334444")

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || !merged {
		t.Fatalf("promote: err=%v merged=%t, want a merge", err, merged)
	}
	if contact.Id != existing.Id {
		t.Fatalf("merged into %s, want the existing contact %s", contact.Id, existing.Id)
	}

	numbers := e.contactNumbers(t, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
	if len(numbers) != 1 || numbers[0] != "+493033334444" {
		t.Errorf("contact numbers = %v, want the number the lead brought with it", numbers)
	}
}
