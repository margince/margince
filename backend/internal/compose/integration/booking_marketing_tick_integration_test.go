// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The booking form's marketing tick asks a question; it never records an answer.
//
// This is the property the whole shape exists for. The public form is
// anonymous: posting it proves only that the caller knows an email address. So
// a tick recorded as consent would let a stranger subscribe somebody else, and
// the form was confined to one operational purpose for exactly that reason.
//
// What a tick buys instead is one confirmation link, minted against the address
// on the CONTACT'S OWN record and mailed there. The grant exists only if that
// mailbox answers. These tests hold both halves: the link appears, and the
// grant does not.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// seededMarketingPurposeID resolves the seeded double-opt-in purpose the tick
// names. It asserts requires_double_opt_in rather than trusting the key,
// because that flag is what the admission rule actually keys on: a seed that
// stopped setting it would make every assertion below pass for the wrong
// reason, with the tick admitted by a rule that no longer guards anything.
func seededMarketingPurposeID(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var id string
	var requiresDOI bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id, requires_double_opt_in FROM consent_purpose
		  WHERE key = 'marketing_email' AND archived_at IS NULL`).Scan(&id, &requiresDOI); err != nil {
		t.Fatalf("reading the seeded marketing purpose: %v", err)
	}
	if !requiresDOI {
		t.Fatal("the seeded marketing purpose does not require double opt-in, " +
			"so admitting the tick would prove nothing about the rule that guards it")
	}
	return id
}

func TestAPublicBookingMarketingTickMailsALinkAndGrantsNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	body := AnyMap{
		"start": monday.Add(3 * time.Hour), "end": monday.Add(210 * time.Minute),
		"booker": AnyMap{"name": "Tessa Ticker", "email": "tessa@visitor.example"},
		"consent": AnyMap{
			"purpose_id": transactional, "policy_version": "pp-2026-01",
			"wording": "You agree we may contact you about this meeting.",
			"marketing": AnyMap{
				"purpose_id": marketing, "policy_version": "mk-2026-01",
				"wording": "Send me your newsletter.",
			},
		},
	}
	if status := publicCall(t, e, "POST", base, body, nil, nil); status != http.StatusCreated {
		t.Fatalf("a booking carrying a marketing tick → %d, want 201", status)
	}

	contactID := contactIDByEmail(t, e, "tessa@visitor.example")

	// The question was asked: a live consent link naming THIS purpose. The
	// purpose rides the token because that is what stops one link granting a
	// different subscription than the mail described.
	var links int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM confirm_token
		 WHERE contact_id = $1 AND purpose_id = $2
		   AND kind = 'consent_confirmation' AND consumed_at IS NULL`,
		contactID, marketing).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("the tick minted %d consent links, want exactly 1", links)
	}

	// And the answer was NOT recorded. This is the assertion the whole design
	// rests on: a stranger who knows an address may cause one mail to be sent
	// to its owner, never a subscription in their name.
	assertNoMarketingGrant(t, e, contactID, marketing)
}

// A tick naming a purpose the form may not ask about is refused BEFORE the
// contact is ensured — the same before-any-write rule the operational half
// follows, and for the same reason: this door is anonymous, so a refusal after
// the ensure would grow the contact table one rejected request at a time.
func TestAPublicBookingRefusesAnInadmissibleMarketingTickBeforeAnyWrite(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	monday := nextMonday()

	// The operational purpose is tracked and live, and still not something a
	// marketing tick may name: it is not confirmed by double opt-in, so a tick
	// naming it would be asking a question no link can pose.
	tick := func(purposeID, version, wording string) AnyMap {
		return AnyMap{
			"start": monday.Add(4 * time.Hour), "end": monday.Add(270 * time.Minute),
			"booker": AnyMap{"name": "Rex Refused", "email": "rex@visitor.example"},
			"consent": AnyMap{
				"purpose_id": transactional, "policy_version": "pp-2026-01",
				"wording": "You agree we may contact you about this meeting.",
				"marketing": AnyMap{
					"purpose_id": purposeID, "policy_version": version, "wording": wording,
				},
			},
		}
	}

	// Each case names the field its refusal must point at. Asserting only the
	// 422 would let two of these collapse onto one rule — or let a reordering
	// swallow the purpose cases behind the wording check — and the test would
	// still pass with four spellings of one refusal.
	marketing := seededMarketingPurposeID(t, e)
	cases := []struct {
		name, field string
		body        AnyMap
	}{
		{
			"a non-double-opt-in purpose", "consent.marketing.purpose_id",
			tick(transactional, "mk-2026-01", "Send me your newsletter."),
		},
		{
			"an untracked purpose", "consent.marketing.purpose_id",
			tick("018f0000-0000-7000-8000-000000000000", "mk-2026-01", "Send me your newsletter."),
		},
		{
			"no wording version", "consent.marketing.policy_version",
			tick(marketing, "", "Send me your newsletter."),
		},
		{
			"no wording", "consent.marketing.wording",
			tick(marketing, "mk-2026-01", ""),
		},
	}
	for _, c := range cases {
		var problem struct {
			Details struct {
				Errors []struct {
					Field string `json:"field"`
				} `json:"errors"`
			} `json:"details"`
		}
		if status := publicCall(t, e, "POST", base, c.body, nil, &problem); status != 422 {
			t.Fatalf("a marketing tick with %s → %d, want 422", c.name, status)
		}
		named := false
		for _, f := range problem.Details.Errors {
			if f.Field == c.field {
				named = true
			}
		}
		if !named {
			t.Fatalf("a marketing tick with %s refused without naming %s (got %+v)",
				c.name, c.field, problem.Details.Errors)
		}
	}

	// Nothing was written by any of them: no contact, no meeting, no link.
	//
	// The counts are taken as a DELTA against a successful booking rather than
	// against zero. On a fresh workspace every one of these tables is already
	// empty, so asserting zero would hold even if the whole admission were
	// deleted — it would be asserting an empty table, not a refusal.
	// A DIFFERENT booker and a free slot: the control must not be the thing
	// that creates Rex, or his absence below would prove nothing.
	control := tick(marketing, "mk-2026-01", "Send me your newsletter.")
	control["booker"] = AnyMap{"name": "Cora Control", "email": "cora@visitor.example"}
	control["start"] = monday.Add(9 * time.Hour)
	control["end"] = monday.Add(570 * time.Minute)

	before := bookingCounts(t, e)
	if status := publicCall(t, e, "POST", base, control, nil, nil); status != http.StatusCreated {
		t.Fatalf("the control booking → %d, want 201", status)
	}
	after := bookingCounts(t, e)
	if after.contacts <= before.contacts || after.meetings <= before.meetings || after.links <= before.links {
		t.Fatalf("the control booking moved nothing (%+v → %+v), so the counts below prove nothing",
			before, after)
	}

	// Rex is the address only the REFUSED bodies carried, so his absence is the
	// refusals leaving nothing behind rather than the table being empty.
	var rex int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM contact_email WHERE email = 'rex@visitor.example'`).Scan(&rex); err != nil {
		t.Fatal(err)
	}
	if rex != 0 {
		t.Fatalf("refused ticks left %d contact rows behind for the refused booker, want 0", rex)
	}
}

// bookingCounts is the three tables a booking touches, read together.
type counts struct{ contacts, meetings, links int }

func bookingCounts(t *testing.T, e *apptest.AppEnv) counts {
	t.Helper()
	var c counts
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM contact),
		       (SELECT count(*) FROM activity WHERE kind = 'meeting'),
		       (SELECT count(*) FROM confirm_token)`).Scan(&c.contacts, &c.meetings, &c.links); err != nil {
		t.Fatal(err)
	}
	return c
}

// A booking with no tick is exactly the booking it was before this feature: the
// operational grant lands and nothing else moves. Without this the suite could
// not tell "the tick works" from "every booking now mails a link".
func TestAPublicBookingWithoutAMarketingTickMailsNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	body := AnyMap{
		"start": monday.Add(5 * time.Hour), "end": monday.Add(330 * time.Minute),
		"booker": AnyMap{"name": "Quinn Quiet", "email": "quinn@visitor.example"},
		"consent": AnyMap{
			"purpose_id": transactional, "policy_version": "pp-2026-01",
			"wording": "You agree we may contact you about this meeting.",
		},
	}
	if status := publicCall(t, e, "POST", base, body, nil, nil); status != http.StatusCreated {
		t.Fatalf("a booking with no marketing tick → %d, want 201", status)
	}

	contactID := contactIDByEmail(t, e, "quinn@visitor.example")

	var links int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE contact_id = $1`, contactID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatalf("a booking with no tick minted %d confirm links, want 0", links)
	}
	assertNoMarketingGrant(t, e, contactID, marketing)

	// The operational grant it DID carry is untouched, so the absence above is
	// the tick's absence and not a booking that failed to record anything.
	var state string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT state FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2`,
		contactID, transactional).Scan(&state); err != nil {
		t.Fatalf("reading the operational grant: %v", err)
	}
	if state != "granted" {
		t.Fatalf("operational consent state = %q, want granted", state)
	}
}

// contactIDByEmail resolves the contact the anonymous booker was ensured into.
func contactIDByEmail(t *testing.T, e *apptest.AppEnv, email string) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT contact_id FROM contact_email WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatalf("resolving the booker %q: %v", email, err)
	}
	return id
}

// assertNoMarketingGrant proves the subject holds no marketing consent and no
// proof event for it — both, because a state row and an event row are written
// by different statements and either alone would be a partial grant.
func assertNoMarketingGrant(t *testing.T, e *apptest.AppEnv, contactID, purposeID string) {
	t.Helper()
	var states, events int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2),
		       (SELECT count(*) FROM consent_event  WHERE contact_id = $1 AND purpose_id = $2)`,
		contactID, purposeID).Scan(&states, &events); err != nil {
		t.Fatal(err)
	}
	if states != 0 || events != 0 {
		t.Fatalf("the tick recorded %d consent states and %d proof events, want none — "+
			"a tick asks a question, and only the subject's own mailbox may answer it",
			states, events)
	}
}

// A tick submitted from an address a contact holds but does NOT read
// confirmations at is refused, and mails nothing.
//
// This is not hypothetical: it was found by probing this path rather than
// reading it. EnsureContactByEmail resolves the booking email to ANY existing
// contact holding it, including on a non-primary address, and the mint then
// derives the destination from that contact's PRIMARY. The link therefore
// arrived at an address the form never named, asking its holder to confirm a
// subscription requested from an address they had stopped using.
//
// Deriving the destination from the record stays right — a caller who could
// name it could name a stranger's. What the guard adds is that the two
// addresses must AGREE, because when they disagree neither the mint nor the
// subject can tell who actually asked.
func TestAMarketingTickFromANonPrimaryAddressIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Vera Verified",
		"emails": []AnyMap{
			{"email": "vera@corp.example", "email_type": "work", "is_primary": true},
			{"email": "vera-old@corp.example", "email_type": "work", "is_primary": false},
		},
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}

	tick := func(email string) AnyMap {
		return AnyMap{
			"start": monday.Add(6 * time.Hour), "end": monday.Add(390 * time.Minute),
			"booker": AnyMap{"name": "Vera Verified", "email": email},
			"consent": AnyMap{
				"purpose_id": transactional, "policy_version": "pp-2026-01",
				"wording": "You agree we may contact you about this meeting.",
				"marketing": AnyMap{
					"purpose_id": marketing, "policy_version": "mk-2026-01",
					"wording": "Send me your newsletter.",
				},
			},
		}
	}

	if status := publicCall(t, e, "POST", base, tick("vera-old@corp.example"), nil, nil); status != 422 {
		t.Fatalf("a tick from a non-primary address → %d, want 422", status)
	}
	var links int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE contact_id = $1`, contact.ID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatalf("a refused tick minted %d links, want 0 — the refusal must mail nothing", links)
	}

	// The SAME contact ticking from the address their confirmations reach is
	// admitted. Without this the refusal above would be satisfied by a guard
	// that refuses everyone, which is the shape that passes for the wrong
	// reason. It also proves the comparison is case-insensitive: an address
	// differing only in case is the same mailbox.
	if status := publicCall(t, e, "POST", base, tick("VERA@corp.example"), nil, nil); status != http.StatusCreated {
		t.Fatalf("a tick from the primary address (differing in case) → %d, want 201", status)
	}
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE contact_id = $1`, contact.ID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("the admitted tick minted %d links, want 1", links)
	}
}

// A subject who withdrew this purpose is not asked about it again by somebody
// else. Verified as a live defect before the guard existed: the booking
// answered 201 and the link was minted and mailed.
//
// Nothing downstream catches it. The confirmation template's category serves
// the subject, which is the class the withdrawal validator deliberately passes
// so an unsubscribe acknowledgement can reach somebody who just unsubscribed. A
// fresh invitation to resubscribe travels under that same exemption, so it must
// be refused before it is staged.
//
// The operational half one function away already carries this reasoning —
// NeverOverrideExisting, "a decision already on record, above all a withdrawal,
// must stand". The marketing path did not copy it.
func TestAMarketingTickDoesNotReSolicitAWithdrawnSubject(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	marketing := seededMarketingPurposeID(t, e)
	monday := nextMonday()

	newContact := func(name, email string) string {
		t.Helper()
		var p struct {
			ID string `json:"id"`
		}
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
			"full_name": name,
			"emails":    []AnyMap{{"email": email, "email_type": "work", "is_primary": true}},
		}, nil, &p); status != http.StatusCreated {
			t.Fatalf("create %s → %d", name, status)
		}
		return p.ID
	}
	tickAt := func(hours time.Duration, email string) AnyMap {
		return AnyMap{
			"start": monday.Add(hours), "end": monday.Add(hours + 30*time.Minute),
			"booker": AnyMap{"name": "Booker", "email": email},
			"consent": AnyMap{
				"purpose_id": transactional, "policy_version": "pp-2026-01",
				"wording": "You agree we may contact you about this meeting.",
				"marketing": AnyMap{
					"purpose_id": marketing, "policy_version": "mk-2026-01",
					"wording": "Send me your newsletter.",
				},
			},
		}
	}
	linksFor := func(contactID string) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(context.Background(),
			`SELECT count(*) FROM confirm_token WHERE contact_id = $1 AND purpose_id = $2`,
			contactID, marketing).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	withdrawn := newContact("Wendy Withdrawn", "wendy@corp.example")
	if status := e.Call(t, "POST", "/v1/contacts/"+withdrawn+"/consent",
		AnyMap{"purpose_id": marketing, "new_state": "withdrawn"}, nil, nil); status != http.StatusOK {
		t.Fatalf("withdrawing the purpose → %d", status)
	}
	if status := publicCall(t, e, "POST", base, tickAt(7*time.Hour, "wendy@corp.example"), nil, nil); status != 422 {
		t.Fatalf("a tick naming a withdrawn subject → %d, want 422", status)
	}
	if n := linksFor(withdrawn); n != 0 {
		t.Fatalf("a withdrawn subject was mailed %d confirmation links, want 0", n)
	}

	// The positive control, and it is not optional: without it this test passes
	// against a guard that refuses EVERY tick, which is the shape that looks
	// like a fix and is a regression. A contact who never answered is still
	// asked.
	fresh := newContact("Fiona Fresh", "fiona@corp.example")
	if status := publicCall(t, e, "POST", base, tickAt(8*time.Hour, "fiona@corp.example"), nil, nil); status != http.StatusCreated {
		t.Fatalf("a tick naming a subject with no decision on file → %d, want 201", status)
	}
	if n := linksFor(fresh); n != 1 {
		t.Fatalf("a subject with no decision on file got %d links, want 1", n)
	}
}
