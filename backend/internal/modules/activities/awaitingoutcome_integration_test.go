// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Which meetings still owe an answer about how they went.
//
// A predicate, so it needs a database: the four status states are four column
// values, and a unit test over the built SQL would assert the string this file
// is here to prove. The one that matters is NULL — capture writes calendar
// events with no status at all, so `meeting_status = 'booked'` reads as
// correct, passes review, and empties this question on exactly the
// installations whose calendars are connected.

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAwaitingOutcomeCarriesTheUnsettledMeetingsAndNoOthers(t *testing.T) {
	e := setupPromises(t)
	booked, captured := ids.NewV7(), ids.NewV7()
	held, noShow, canceled := ids.NewV7(), ids.NewV7(), ids.NewV7()

	// The two that still owe an answer. `captured` carries no status at all,
	// which is what a synced calendar event looks like before anybody touches
	// it — the row this predicate exists to keep.
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Rollout review', now() - interval '2 hours', 'booked', 'seed', 'system')`, booked)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Synced from the calendar', now() - interval '3 hours', 'seed', 'system')`, captured)

	// The three that have been answered, one per settled state. Each is a
	// separate arm of the same OR, and a predicate that admitted any of them
	// would put a finished meeting back on a queue asking how it went.
	for id, status := range map[ids.UUID]string{held: "held", noShow: "no_show", canceled: "canceled"} {
		e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
			VALUES ($1, 'meeting', 'Settled', now() - interval '4 hours', $2, 'seed', 'system')`, id, status)
	}

	kind := "meeting"
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	got, _, err := store.ListActivities(e.as(), ListActivitiesInput{
		Kind: &kind, AwaitingOutcome: true,
	})
	if err != nil {
		t.Fatalf("reading the meetings awaiting an outcome: %v", err)
	}
	carried := map[ids.UUID]bool{}
	for _, row := range got {
		carried[ids.UUID(row.Id)] = true
	}
	for name, id := range map[string]ids.UUID{"a booked meeting": booked, "a captured meeting with no status": captured} {
		if !carried[id] {
			t.Errorf("%s is not carried, so the lane would report nothing to do over real work", name)
		}
	}
	for name, id := range map[string]ids.UUID{"held": held, "no_show": noShow, "canceled": canceled} {
		if carried[id] {
			t.Errorf("a %s meeting is carried, so a settled meeting is asked about again", name)
		}
	}
}

// And the dial is a dial: without it the same read carries everything.
//
// Without this the test above passes over a store that ignores the field
// entirely — every meeting would be "awaiting", including the three settled
// ones, and only the second half of that assertion would catch it. This says
// the difference is the flag rather than the fixture.
func TestWithoutTheDialEveryMeetingIsCarried(t *testing.T) {
	e := setupPromises(t)
	booked, held := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Rollout review', now() - interval '2 hours', 'booked', 'seed', 'system')`, booked)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Settled', now() - interval '4 hours', 'held', 'seed', 'system')`, held)

	kind := "meeting"
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	got, _, err := store.ListActivities(e.as(), ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatalf("reading every meeting: %v", err)
	}
	carried := map[ids.UUID]bool{}
	for _, row := range got {
		carried[ids.UUID(row.Id)] = true
	}
	if !carried[booked] || !carried[held] {
		t.Fatal("the unfiltered read is missing a meeting, so the fixture rather than the " +
			"dial is what the test above measures")
	}
}

// A customer follow-up is owed for a meeting with a customer: one linked to a
// contact, company or lead. An internal stand-up owes none.
func TestOnlyACustomerMeetingOwesAFollowUp(t *testing.T) {
	e := setupPromises(t)
	standUp, customer, contact := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Daily stand-up', now() - interval '2 hours', 'seed', 'system')`, standUp)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Customer review', now() - interval '2 hours', 'seed', 'system')`, customer)
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Buyer', 'seed', 'system')`, contact)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`,
		ids.NewV7(), customer, contact)

	kind := "meeting"
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	got, _, err := store.ListActivities(e.as(), ListActivitiesInput{
		Kind: &kind, AwaitingOutcome: true, CustomerMeetingsOnly: true,
	})
	if err != nil {
		t.Fatalf("reading the customer meetings awaiting an outcome: %v", err)
	}
	carried := map[ids.UUID]bool{}
	for _, row := range got {
		carried[ids.UUID(row.Id)] = true
	}
	if !carried[customer] {
		t.Error("a customer meeting owing an outcome is not carried")
	}
	if carried[standUp] {
		t.Error("an internal stand-up is carried as customer follow-up")
	}
}
