// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The three-lane exact ladder over a real migrated Postgres: a channel
// binding and an E.164 phone each resolve on their own key; the channel lane
// outranks email; a blocked identity still resolves (blocked_at is
// reachability, not identity); and when two lanes name different contacts the
// routing is deterministic, the rival is reported, and NOTHING is written
// onto the rival — preferring a lane routes, writing keys merges.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const telegramProvider = "telegram"

// seedContact creates a contact carrying the given emails and phones — the
// incumbent each lane probes against.
func (e *dedupeEnv) seedContact(ctx context.Context, t *testing.T, name string, emails, phones []string) ids.ContactID {
	t.Helper()
	in := CreateContactInput{FullName: name, Source: "manual"}
	for i, addr := range emails {
		in.Emails = append(in.Emails, ContactEmailInput{Email: addr, EmailType: "work", IsPrimary: i == 0, Position: i + 1})
	}
	for i, phone := range phones {
		in.Phones = append(in.Phones, ContactPhoneInput{Phone: phone, PhoneType: "mobile", IsPrimary: i == 0, Position: i + 1})
	}
	contact, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatalf("seed contact %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

// bindIdentity binds one channel identity through the real write path, so the
// tests exercise the same insert-then-adopt the ingress will.
func (e *dedupeEnv) bindIdentity(ctx context.Context, t *testing.T, contactID ids.ContactID, ci connector.ChannelIdentity) ids.ContactID {
	t.Helper()
	var bound ids.ContactID
	err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		bound, err = ResolveOrCreateChannelIdentity(ctx, tx, contactID, ci)
		return err
	})
	if err != nil {
		t.Fatalf("bind channel identity %s: %v", ci.ChannelUserID, err)
	}
	return bound
}

// blockIdentity is what my_chat_member does when the user blocks the bot: it
// sets reachability, and touches nothing about identity.
func (e *dedupeEnv) blockIdentity(ctx context.Context, t *testing.T, ci connector.ChannelIdentity) {
	t.Helper()
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE contact_channel_identity SET blocked_at = now()
			WHERE provider = $1 AND channel_user_id = $2 AND archived_at IS NULL`,
			ci.Provider, ci.ChannelUserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Errorf("blocking %s touched %d rows, want 1", ci.ChannelUserID, tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("block channel identity: %v", err)
	}
}

// assertNoChannelIdentityFor is the merge guard: a lane that lost the routing
// decision must not have gained the winner's key. If it ever does, two real
// humans have quietly become one record.
func assertNoChannelIdentityFor(ctx context.Context, t *testing.T, tx pgx.Tx, contactID ids.ContactID) {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM contact_channel_identity WHERE contact_id = $1`, contactID).Scan(&n); err != nil {
		t.Fatalf("counting channel identities for %s: %v", contactID, err)
	}
	if n != 0 {
		t.Fatalf("the rival %s carries %d channel identity rows; resolution must never write a key onto it", contactID, n)
	}
}

func TestExactContactByChannelIdentityMatchesOnProviderAndUserID(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	jane := e.seedContact(ctx, t, "Jane Doe", []string{"jane@ci.test"}, nil)
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770001", Username: "janedoe"}
	e.bindIdentity(ctx, t, jane, ci)

	// The binding is the key: neither the name nor an unknown address on the
	// candidate may change where an inbound message lands.
	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "unrelated stranger", ChannelIdentities: []connector.ChannelIdentity{ci},
	})
	if r.Decision != DecisionExactCollision {
		t.Fatalf("decision = %s, want exact_collision", r.Decision)
	}
	if r.ContactID != jane {
		t.Fatalf("resolved %s, want the bound contact %s", r.ContactID, jane)
	}
	if r.Conflict != nil {
		t.Fatalf("unexpected conflict %+v — only one lane carried a key", r.Conflict)
	}

	// A different channel user id on the same provider is a different human.
	other := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Jane Doe",
		ChannelIdentities: []connector.ChannelIdentity{
			{Provider: telegramProvider, ChannelUserID: "770002"},
		},
	})
	if other.ContactID == jane {
		t.Fatal("channel user 770002 resolved onto the contact bound to 770001")
	}
}

func TestExactContactByPhoneMatchesOnE164(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	sam := e.seedContact(ctx, t, "Sam Reed", []string{"sam@ph.test"}, []string{"+4915100000001"})

	// The candidate arrives in a different notation of the same number; the
	// lane compares the E.164 form contact_phone actually stores.
	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "S. Reed", Phones: []string{"0049 151 000 000 01"},
	})
	if r.Decision != DecisionExactCollision {
		t.Fatalf("decision = %s, want exact_collision", r.Decision)
	}
	if r.ContactID != sam {
		t.Fatalf("resolved %s, want %s", r.ContactID, sam)
	}

	// A number that cannot be normalized is no key: it is dropped, and the
	// ladder falls through instead of matching something by accident.
	unusable := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Nobody Here", Phones: []string{"151-000-000-01"},
	})
	if unusable.Decision != DecisionNoMatch {
		t.Fatalf("decision = %s (contact %s), want no_match for an un-normalizable number",
			unusable.Decision, unusable.ContactID)
	}
}

func TestLadderPrefersChannelIdentityOverEmail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	bound := e.seedContact(ctx, t, "Bound Contact", []string{"bound@ladder.test"}, nil)
	byEmail := e.seedContact(ctx, t, "Email Contact", []string{"shared@ladder.test"}, nil)
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770101"}
	e.bindIdentity(ctx, t, bound, ci)

	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName:          "Bound Contact",
		Emails:            []string{"shared@ladder.test"},
		ChannelIdentities: []connector.ChannelIdentity{ci},
	})
	if r.ContactID != bound {
		t.Fatalf("resolved %s, want the channel-bound contact %s — an established binding outranks a shared address", r.ContactID, bound)
	}
	if r.Conflict == nil {
		t.Fatal("two lanes named different contacts and no conflict was reported")
	}
	if r.Conflict.RoutedLane != laneChannelIdentity || r.Conflict.RivalLane != LaneEmail {
		t.Fatalf("conflict lanes = %s over %s, want %s over %s",
			r.Conflict.RoutedLane, r.Conflict.RivalLane, laneChannelIdentity, LaneEmail)
	}
	if r.Conflict.Rival != byEmail {
		t.Fatalf("rival = %s, want the email lane's contact %s", r.Conflict.Rival, byEmail)
	}
}

func TestBlockedIdentityStillResolves(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	returning := e.seedContact(ctx, t, "Returning Customer", []string{"back@blocked.test"}, nil)
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770201"}
	e.bindIdentity(ctx, t, returning, ci)
	e.blockIdentity(ctx, t, ci)

	// blocked_at is reachability, not identity. A lane that honoured it would
	// miss here and — with no email and no phone on the candidate — fork this
	// human into a second contact the moment they unblock and write again.
	r := e.dedupeInTx(ctx, t, ContactCandidate{
		ChannelIdentities: []connector.ChannelIdentity{ci},
	})
	if r.Decision != DecisionExactCollision {
		t.Fatalf("decision = %s, want exact_collision — blocking must not unbind an identity", r.Decision)
	}
	if r.ContactID != returning {
		t.Fatalf("resolved %s, want %s", r.ContactID, returning)
	}
}

func TestLaneConflictRoutesDeterministicallyAndReportsRival(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactA := e.seedContact(ctx, t, "Contact A", []string{"a@conflict.test"}, nil)
	contactB := e.seedContact(ctx, t, "Contact B", []string{"b@conflict.test"}, []string{"+4915100000042"})
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770301"}
	e.bindIdentity(ctx, t, contactA, ci)

	candidate := ContactCandidate{
		FullName:          "Contact B",
		Phones:            []string{"+4915100000042"},
		ChannelIdentities: []connector.ChannelIdentity{ci},
	}
	var r ContactResolution
	err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		if r, err = DedupeContact(ctx, tx, candidate); err != nil {
			return err
		}
		// Asserted inside the resolver's own transaction: if resolution wrote
		// anything onto the rival, it is visible here and nowhere else.
		assertNoChannelIdentityFor(ctx, t, tx, contactB)
		return nil
	})
	if err != nil {
		t.Fatalf("DedupeContact: %v", err)
	}
	if r.ContactID != contactA {
		t.Fatalf("routed to %s, want %s — routing must be deterministic, never deferred", r.ContactID, contactA)
	}
	if r.Conflict == nil {
		t.Fatal("the identity lane and the phone lane named different contacts; the conflict must be reported")
	}
	if r.Conflict.RoutedTo != contactA || r.Conflict.Rival != contactB {
		t.Fatalf("conflict = routed %s / rival %s, want routed %s / rival %s",
			r.Conflict.RoutedTo, r.Conflict.Rival, contactA, contactB)
	}
	if r.Conflict.RoutedLane != laneChannelIdentity || r.Conflict.RivalLane != lanePhone {
		t.Fatalf("conflict lanes = %s over %s, want %s over %s",
			r.Conflict.RoutedLane, r.Conflict.RivalLane, laneChannelIdentity, lanePhone)
	}
}
