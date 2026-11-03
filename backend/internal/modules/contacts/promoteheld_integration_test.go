// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Somebody is a lead or a contact, never both. A contact typed in over a live
// lead's address converts the lead only when asked. Mail with a live lead
// makes no contact at all: it files on the lead, and nobody converts by writing.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadOutcome reads what became of a lead: its status, the contact it was
// promoted to, whether it is archived, and the trigger its promote audit row
// recorded ("" when there is none).
func (e *dedupeEnv) leadOutcome(ctx context.Context, t *testing.T, lead ids.UUID) (status string, promotedTo *ids.UUID, archived bool, trigger string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT status, promoted_contact_id, archived_at IS NOT NULL FROM lead WHERE id = $1`,
			lead).Scan(&status, &promotedTo, &archived); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT coalesce(max(after->>'trigger'), '') FROM audit_log
			  WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'promote'`,
			lead).Scan(&trigger)
	}); err != nil {
		t.Fatal(err)
	}
	return status, promotedTo, archived, trigger
}

func (e *dedupeEnv) seedHeldLead(ctx context.Context, t *testing.T, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO lead (id, full_name, email, status, source, source_system, captured_by)
			 VALUES ($1, 'Held Lead', lower($2), 'new', 'hubspot_import:1', 'mirror:hubspot', 'human:x')`,
			id, email)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// Typing in a contact for somebody who is already a lead promotes the lead into
// that contact: archived as promoted, pointing at the contact, and the contact
// pointing back.
func TestCreatingAContactConvertsTheLeadHoldingItsAddressOnlyWhenAsked(t *testing.T) {
	e := setupPromoteConsent(t)
	const email = "typed@held.test"
	lead := e.seedLead(t, email)
	activity := e.seedLeadActivity(t, lead, "Met at the trade fair")
	in := CreateContactInput{
		FullName: "Typed In",
		Emails:   []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	}
	if _, err := e.store.CreateContact(e.ctx, in); !errors.As(err, new(*LeadHoldsIdentityError)) {
		t.Fatalf("a create over a live lead without the convert: %v", err)
	}
	in.ConvertLeadIDs = []ids.LeadID{lead}
	contact, err := e.store.CreateContact(e.ctx, in)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	var status string
	var promotedTo *ids.UUID
	var archived bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, promoted_contact_id, archived_at IS NOT NULL FROM lead WHERE id = $1`,
		lead.UUID).Scan(&status, &promotedTo, &archived); err != nil {
		t.Fatal(err)
	}
	if status != "promoted" || !archived || promotedTo == nil || *promotedTo != ids.UUID(contact.Id) {
		t.Fatalf("lead is status=%q archived=%t promoted_to=%v, want promoted and archived onto contact %v",
			status, archived, promotedTo, contact.Id)
	}
	if contact.ConvertedFromLeadId == nil || ids.UUID(*contact.ConvertedFromLeadId) != lead.UUID {
		t.Errorf("the created contact reads converted_from_lead_id=%v, want the lead %v — "+
			"the response must show the merge it just made", contact.ConvertedFromLeadId, lead.UUID)
	}
	if got := e.linkTargetsOf(t, activity)["contact"]; got != ids.UUID(contact.Id) {
		t.Errorf("the lead's activity links contact %v, want %v: its history stayed behind", got, contact.Id)
	}
}

// liveContactsHolding counts the live contacts holding an address.
func (e *dedupeEnv) liveContactsHolding(ctx context.Context, t *testing.T, email string) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact c
			JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
			WHERE ce.email = lower($1) AND c.archived_at IS NULL`, email).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// requireLeadUntouched fails unless the lead is still a live, unpromoted lead.
func (e *dedupeEnv) requireLeadUntouched(ctx context.Context, t *testing.T, lead ids.UUID, when string) {
	t.Helper()
	if status, promotedTo, archived, _ := e.leadOutcome(ctx, t, lead); status == "promoted" || archived || promotedTo != nil {
		t.Fatalf("%s, lead is status=%q archived=%t promoted_to=%v, want it untouched", when, status, archived, promotedTo)
	}
}

// Mail from somebody who is a lead files on the lead. No contact is made, and
// the lead stays a lead although they wrote to us.
func TestCapturedMailFromALeadFilesOnTheLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "wrote@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-time.Hour).UTC())

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if res.ContactCreated || !res.ContactID.IsZero() || res.LeadID.UUID != lead {
		t.Fatalf("ensure = %+v, want no contact and the lead %v", res, lead)
	}
	if n := e.liveContactsHolding(ctx, t, email); n != 0 {
		t.Errorf("%d live contacts hold a live lead's address after mail, want none", n)
	}
	e.requireLeadUntouched(ctx, t, lead, "after their reply")
}

// The verdict path ensures without owner scope. A lead's address still makes
// no contact there.
func TestAWorkspaceEnsureOfALeadMakesNoContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "accepted@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-time.Hour).UTC())

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || res.LeadID.UUID != lead {
		t.Fatalf("ensure = %+v (err %v), want the lead %v", res, err, lead)
	}
	if n := e.liveContactsHolding(ctx, t, email); n != 0 {
		t.Errorf("%d live contacts hold the lead's address, want none", n)
	}
	e.requireLeadUntouched(ctx, t, lead, "after a workspace ensure")
}

// Capture keeps an unjudged sender's contact to its owner. For a lead there is
// no such contact, before the verdict or after it.
func TestAnOwnerScopedCaptureOfALeadMakesNoContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "unjudged@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-time.Hour).UTC())
	in.NarrowedBecause = NarrowedAwaitingVerdict

	for _, scoped := range []bool{true, false} {
		in.OwnerScoped = scoped
		if !scoped {
			in.NarrowedBecause = ""
		}
		res, err := e.store.EnsureCounterparty(ctx, in)
		if err != nil || res.ContactCreated || res.LeadID.UUID != lead {
			t.Fatalf("ensure (owner scoped %t) = %+v (err %v), want the lead %v and no contact", scoped, res, err, lead)
		}
	}
	if n := e.liveContactsHolding(ctx, t, email); n != 0 {
		t.Errorf("%d live contacts hold the lead's address, want none", n)
	}
	e.requireLeadUntouched(ctx, t, lead, "after both ensures")
}

// Mail we sent to a lead makes no contact, and nor does their reply.
func TestOutboundMailToALeadMakesNoContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "cold@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	sent := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-2*time.Hour).UTC()))

	outbound, err := e.store.EnsureCounterparty(ctx, sent)
	if err != nil || outbound.ContactCreated || outbound.LeadID.UUID != lead {
		t.Fatalf("outbound ensure = %+v (err %v), want the lead %v", outbound, err, lead)
	}
	reply := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-time.Hour).UTC())
	if res, err := e.store.EnsureCounterparty(ctx, reply); err != nil || res.ContactCreated {
		t.Fatalf("reply ensure = %+v (err %v), want no contact", res, err)
	}
	if n := e.liveContactsHolding(ctx, t, email); n != 0 {
		t.Errorf("%d live contacts hold the lead's address, want none", n)
	}
	e.requireLeadUntouched(ctx, t, lead, "after their reply")
}

// A historical lead/contact pair stays as it is. A reply lands on the contact
// and converts neither record.
func TestMailToAContactBesideALeadLeavesTheLeadAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "pair@held.test"
	in := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-2*time.Hour).UTC())
	first, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !first.ContactCreated {
		t.Fatalf("first ensure = %+v (err %v), want a created contact", first, err)
	}
	lead := e.seedHeldLead(ctx, t, email)

	reply := e.datedEnsureInput(ctx, t, email, "held.test", clocktest.Now(t).Add(-time.Hour).UTC())
	res, err := e.store.EnsureCounterparty(ctx, reply)
	if err != nil || res.ContactID != first.ContactID || !res.LeadID.IsZero() {
		t.Fatalf("reply ensure = %+v (err %v), want the contact %v", res, err, first.ContactID)
	}
	e.requireLeadUntouched(ctx, t, lead, "after a reply to the contact")
}
