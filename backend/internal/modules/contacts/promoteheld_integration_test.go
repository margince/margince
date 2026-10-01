// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A contact created for an address a live lead holds takes the lead with it.
//
// Before this, the two stood side by side: the lead kept the history and sat on
// the lead list as if nobody had spoken to them, and the contact held the
// conversation. Mail capture made it routine — a lead carried over from the
// previous CRM writes to a connected mailbox, capture mints a contact, and
// nothing connects the two.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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
func TestCreatingAContactPromotesTheLeadHoldingItsAddress(t *testing.T) {
	e := setupPromoteConsent(t)
	const email = "typed@held.test"
	lead := e.seedLead(t, email)
	activity := e.seedLeadActivity(t, lead, "Met at the trade fair")

	contact, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: "Typed In",
		Emails:   []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
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

// Mail from somebody who is a lead mints a contact and promotes the lead into it
// as an inbound reply, with the message as evidence.
func TestCapturedMailFromALeadPromotesTheLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "wrote@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", time.Now().Add(-time.Hour).UTC())

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	status, promotedTo, archived, trigger := e.leadOutcome(ctx, t, lead)
	if status != "promoted" || !archived || promotedTo == nil || *promotedTo != res.ContactID.UUID {
		t.Fatalf("lead is status=%q archived=%t promoted_to=%v, want promoted onto the captured contact %v",
			status, archived, promotedTo, res.ContactID)
	}
	if trigger != string(TriggerInboundReply) {
		t.Errorf("promotion trigger = %q, want %q: they wrote to us", trigger, TriggerInboundReply)
	}
}

// A contact capture keeps to its owner while the sender is unjudged receives no
// workspace lead: folding the lead in would hide it from everybody else.
func TestAnOwnerScopedCapturedContactLeavesTheLeadAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "unjudged@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", time.Now().Add(-time.Hour).UTC())
	in.OwnerScoped = true
	in.NarrowedBecause = NarrowedAwaitingVerdict

	if _, err := e.store.EnsureCounterparty(ctx, in); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if status, promotedTo, archived, _ := e.leadOutcome(ctx, t, lead); status == "promoted" || archived || promotedTo != nil {
		t.Fatalf("lead is status=%q archived=%t promoted_to=%v, want it untouched beside an owner-scoped contact",
			status, archived, promotedTo)
	}

	// The verdict that makes the contact the workspace's hands it the lead.
	in.OwnerScoped = false
	in.NarrowedBecause = ""
	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil {
		t.Fatalf("workspace ensure: %v", err)
	}
	if status, promotedTo, _, _ := e.leadOutcome(ctx, t, lead); status != "promoted" || promotedTo == nil || *promotedTo != res.ContactID.UUID {
		t.Fatalf("after the workspace verdict the lead is status=%q promoted_to=%v, want promoted onto %v",
			status, promotedTo, res.ContactID)
	}
}

// Mail we sent that nobody answered is cold outbound, which promotion refuses:
// the lead stays. Their first reply promotes it into the contact the outbound
// mail already minted.
func TestOutboundMailLeavesTheLeadUntilTheyReply(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "cold@held.test"
	lead := e.seedHeldLead(ctx, t, email)

	sent := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, occurred_at,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'intro', 'outbound', now() - interval '2 hours', 'gmail', $2, 'gmail:seed', 'connector:gmail')`,
			sent, sent.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', $2)`, sent, email)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	outbound, err := e.store.EnsureCounterparty(ctx, EnsureCounterpartyInput{
		Email: email, Domain: "held.test", OwnerID: e.rep, ActivityID: sent,
		Source: "gmail:" + sent.String(), CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("outbound ensure: %v", err)
	}
	if status, promotedTo, archived, _ := e.leadOutcome(ctx, t, lead); status == "promoted" || archived || promotedTo != nil {
		t.Fatalf("after outbound mail only, lead is status=%q archived=%t promoted_to=%v, want it untouched",
			status, archived, promotedTo)
	}

	reply := e.datedEnsureInput(ctx, t, email, "held.test", time.Now().Add(-time.Hour).UTC())
	if _, err := e.store.EnsureCounterparty(ctx, reply); err != nil {
		t.Fatalf("reply ensure: %v", err)
	}
	status, promotedTo, _, trigger := e.leadOutcome(ctx, t, lead)
	if status != "promoted" || promotedTo == nil || *promotedTo != outbound.ContactID.UUID {
		t.Fatalf("after the reply, lead is status=%q promoted_to=%v, want promoted onto %v",
			status, promotedTo, outbound.ContactID)
	}
	if trigger != string(TriggerInboundReply) {
		t.Errorf("promotion trigger = %q, want %q", trigger, TriggerInboundReply)
	}
}

// Capture keeps a contact it minted from mail nobody has answered to its owner,
// so the ensure hands it no lead. The reply that publishes it hands it the lead.
func TestPublishingAContactOnReplyPromotesTheLead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "published@held.test"
	lead := e.seedHeldLead(ctx, t, email)
	in := e.datedEnsureInput(ctx, t, email, "held.test", time.Now().Add(-time.Hour).UTC())
	in.OwnerScoped = true
	in.NarrowedBecause = NarrowedOutboundNoAnswer

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if status, _, _, _ := e.leadOutcome(ctx, t, lead); status == "promoted" {
		t.Fatal("an owner-scoped contact took the lead")
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		moved, err := e.store.PromoteOnReplyTx(ctx, tx, res.ContactID, e.rep)
		if err != nil || !moved {
			t.Fatalf("publish on reply: moved=%t err=%v", moved, err)
		}
		return e.store.PromoteHeldLeadsOnReplyTx(ctx, tx, res.ContactID)
	}); err != nil {
		t.Fatal(err)
	}
	status, promotedTo, _, trigger := e.leadOutcome(ctx, t, lead)
	if status != "promoted" || promotedTo == nil || *promotedTo != res.ContactID.UUID || trigger != string(TriggerInboundReply) {
		t.Fatalf("after publishing, lead is status=%q promoted_to=%v trigger=%q, want promoted onto %v as %q",
			status, promotedTo, trigger, res.ContactID, TriggerInboundReply)
	}
}
