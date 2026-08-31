// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The direction counts say THAT a contact answered; these two dates say which
// way the conversation is owed. A contact who replied in March and was chased
// in August has a non-zero count in both directions, so the counts alone cannot
// tell "they owe us" from "we owe them" — the pair of dates is what separates
// them, and a follow-up offered on the wrong side of that is a rep writing
// again to someone already waiting on them.
func TestStrengthFoldDatesEachDirectionSeparately(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	person := e.SeedPerson(t, "Dietmar Rietsch", nil)
	employ(t, e, person, org, "Managing Director")

	// TWO inbound messages, not one: with a single reply the newest and the
	// oldest are the same row, so a fold that took either would pass. The
	// anchor a follow-up opens must be the LATEST thing they said.
	firstReply := org360Clock.AddDate(0, 0, -60)
	replied := org360Clock.AddDate(0, 0, -40)
	chased := org360Clock.AddDate(0, 0, -5)
	stale := integration.AccountMailDirectedAt(t, owner, e.WS, "First contact", "inbound", firstReply)
	integration.LinkActivity(t, owner, stale, "person", person)
	inbound := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: your proposal", "inbound", replied)
	integration.LinkActivity(t, owner, inbound, "person", person)
	outbound := integration.AccountMailDirectedAt(t, owner, e.WS, "Following up", "outbound", chased)
	integration.LinkActivity(t, owner, outbound, "person", person)

	got := foldOneContact(t, e, org, person)

	assertSameInstant(t, "last inbound", got.LastInbound, replied)
	assertSameInstant(t, "last outbound", got.LastOutbound, chased)
	// The anchor a follow-up would answer is their message, not our chase.
	if got.LastInboundActivity == nil {
		t.Fatal("no inbound anchor recorded for a contact who wrote in")
	}
	if got.LastInboundActivity.UUID != inbound {
		t.Fatalf("the inbound anchor is %s, want the message they sent (%s)",
			*got.LastInboundActivity, inbound)
	}
	if people.EngagementOf(got) != people.EngagementAnswered {
		t.Fatalf("a contact who wrote in reads as %q", people.EngagementOf(got))
	}
}

// A contact nobody has heard from carries no inbound date and no anchor. The
// absence is the fact: offering a "follow up" on a thread that does not exist
// is the composer opening a reply to nothing.
func TestStrengthFoldLeavesTheInboundAnchorEmptyWhenNobodyWroteIn(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	person := e.SeedPerson(t, "Philipp Königs", nil)
	employ(t, e, person, org, "CFO")

	sent := org360Clock.AddDate(0, 0, -12)
	outbound := integration.AccountMailDirectedAt(t, owner, e.WS, "Introduction", "outbound", sent)
	integration.LinkActivity(t, owner, outbound, "person", person)

	got := foldOneContact(t, e, org, person)

	if got.LastInbound != nil {
		t.Fatalf("a contact who never wrote in carries an inbound date of %s", got.LastInbound)
	}
	if got.LastInboundActivity != nil {
		t.Fatalf("a contact who never wrote in carries an inbound anchor of %s", got.LastInboundActivity)
	}
	assertSameInstant(t, "last outbound", got.LastOutbound, sent)
	if people.EngagementOf(got) != people.EngagementNoReply {
		t.Fatalf("a contact we wrote to with no reply reads as %q", people.EngagementOf(got))
	}
}

// A reply older than the window leaves a DATE but no anchor.
//
// The two are answering different questions. "They last wrote eighteen months
// ago" is history and stays true however old it gets. The anchor is an action —
// it is what a Follow up button opens a reply against — and it has to agree
// with the state the counts report. This contact reads untried, because nothing
// inside the window says otherwise; offering to answer a thread from last year
// would contradict the same page's own summary.
func TestStrengthFoldDropsAnInboundAnchorOlderThanTheWindow(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)

	org := e.SeedOrg(t, "Brandt GmbH", nil)
	person := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, person, org, "Procurement")

	longAgo := org360Clock.AddDate(0, 0, -400)
	stale := integration.AccountMailDirectedAt(t, owner, e.WS, "Re: 2025 tender", "inbound", longAgo)
	integration.LinkActivity(t, owner, stale, "person", person)

	got := foldOneContact(t, e, org, person)

	// The history is kept: the date is what the page prints as "last heard from".
	assertSameInstant(t, "last inbound", got.LastInbound, longAgo)
	// The action is not offered.
	if got.LastInboundActivity != nil {
		t.Fatalf("a reply from %s is outside the 90-day window and must not be offered as a reply anchor, got %s",
			longAgo.Format("2006-01-02"), got.LastInboundActivity)
	}
	if people.EngagementOf(got) != people.EngagementUntried {
		t.Fatalf("a contact whose only message predates the window reads as %q, want untried",
			people.EngagementOf(got))
	}
}

// foldOneContact runs the account roster read and returns the one contact's
// §4 fold — through the real read rather than a hand-built row, so the test
// measures what the page is served.
func foldOneContact(t *testing.T, e *integration.Env, org, person ids.UUID) people.RelationshipStrength {
	t.Helper()
	var found *people.RelationshipStrength
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		all, err := people.StrengthForOrgContacts(ctx, tx, ids.OrganizationID{UUID: org}, org360Clock)
		if err != nil {
			return err
		}
		for _, c := range all {
			if c.PersonID.UUID == person {
				strength := c.Strength
				found = &strength
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the account roster: %v", err)
	}
	if found == nil {
		t.Fatalf("the roster did not carry the seeded contact %s", person)
	}
	return *found
}

func assertSameInstant(t *testing.T, what string, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is absent, want %s", what, want)
	}
	if !got.Equal(want) {
		t.Fatalf("%s is %s, want %s", what, got, want)
	}
}
