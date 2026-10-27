// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// customerMeeting logs a meeting with one customer through the module's writer.
func customerMeeting(t *testing.T, e *sendEnv, customer ids.UUID, subject string, start time.Time, status *string) ids.ActivityID {
	t.Helper()
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start,
		MeetingStatus: status, Source: "manual",
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(out.Id))
}

func seedCustomer(t *testing.T, e *sendEnv) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Buyer', 'seed', 'system')`, id); err != nil {
		t.Fatalf("seeding the customer: %v", err)
	}
	return id
}

// The read's stamp answers the outcome lane's rule without its look-back window.
// A booked meeting a month old still asks, which the lane alone never shows.
func TestTheActivityReadStampsWhichMeetingsAwaitAnOutcome(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	booked, held := "booked", "held"
	now := clocktest.Now(t)
	monthOld := customerMeeting(t, e, customer, "Booked a month ago", now.AddDate(0, 0, -30), &booked)
	captured := customerMeeting(t, e, customer, "Synced, no status", now.Add(-2*time.Hour), nil)
	ahead := customerMeeting(t, e, customer, "Booked for next week", now.AddDate(0, 0, 7), &booked)
	answered := customerMeeting(t, e, customer, "Held yesterday", now.AddDate(0, 0, -1), &held)
	want := map[ids.ActivityID]bool{monthOld: true, captured: true, ahead: false, answered: false}

	kind := string(crmcontracts.ActivityKindMeeting)
	page, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatalf("listing the meetings: %v", err)
	}
	listed := map[ids.ActivityID]bool{}
	for _, row := range page {
		if row.AwaitingOutcome == nil {
			t.Fatalf("%q carries no awaiting_outcome; every read that stamps writable stamps it", *row.Subject)
		}
		listed[ids.From[ids.ActivityKind](ids.UUID(row.Id))] = *row.AwaitingOutcome
	}
	for id, owed := range want {
		if listed[id] != owed {
			t.Errorf("list: meeting %s awaiting_outcome = %v, want %v", id, listed[id], owed)
		}
		one, err := meetingStore(e).GetActivity(meetingCtx(e), id, storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading meeting %s: %v", id, err)
		}
		if one.AwaitingOutcome == nil || *one.AwaitingOutcome != owed {
			t.Errorf("single read: meeting %s awaiting_outcome = %v, want %v", id, one.AwaitingOutcome, owed)
		}
	}
}

// The lane's filter and the read's stamp are one rule: the filter keeps the
// meetings the stamp marks, and no others.
func TestTheOutcomeFilterKeepsWhatTheStampMarks(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	booked, canceled := "booked", "canceled"
	now := clocktest.Now(t)
	customerMeeting(t, e, customer, "Past, booked", now.AddDate(0, 0, -3), &booked)
	customerMeeting(t, e, customer, "Past, canceled", now.AddDate(0, 0, -3), &canceled)
	customerMeeting(t, e, customer, "Ahead, booked", now.AddDate(0, 0, 3), &booked)
	internal := "Stand-up, no customer"
	if _, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &internal, OccurredAt: new(now.Add(-time.Hour)), Source: "manual",
	}); err != nil {
		t.Fatalf("logging the stand-up: %v", err)
	}

	kind := string(crmcontracts.ActivityKindMeeting)
	all, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatalf("listing every meeting: %v", err)
	}
	stamped := map[crmcontracts.Id]bool{}
	for _, row := range all {
		if row.AwaitingOutcome != nil && *row.AwaitingOutcome {
			stamped[row.Id] = true
		}
	}
	kept, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind, AwaitingOutcomeAsOf: &now})
	if err != nil {
		t.Fatalf("listing the meetings awaiting an outcome: %v", err)
	}
	if len(kept) != 1 || len(stamped) != 1 {
		t.Fatalf("the filter kept %d and the stamp marked %d, want the one past booked customer meeting each", len(kept), len(stamped))
	}
	if !stamped[kept[0].Id] {
		t.Errorf("the filter kept %q, which the stamp did not mark", *kept[0].Subject)
	}
}

// Held and no-show describe a meeting that took place, so the patch refuses
// them ahead of the start. Canceling a future meeting stays allowed.
func TestAnOutcomeWaitsForTheMeetingToStart(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	booked := "booked"
	ahead := customerMeeting(t, e, customer, "Booked for tomorrow", clocktest.Now(t).AddDate(0, 0, 1), &booked)

	noShow := string(crmcontracts.ActivityMeetingStatusNoShow)
	_, err := meetingStore(e).UpdateActivity(meetingCtx(e), ahead, UpdateActivityInput{MeetingStatus: &noShow})
	var early *MeetingNotTakenPlaceError
	if !errors.As(err, &early) {
		t.Fatalf("a no-show on tomorrow's meeting answered %v, want the not-taken-place refusal", err)
	}
	if field, code, _ := early.FieldFault(); field != "meeting_status" || code != codeInFuture {
		t.Errorf("the refusal names %s/%s, want meeting_status/%s", field, code, codeInFuture)
	}

	canceled := string(crmcontracts.ActivityMeetingStatusCanceled)
	out, err := meetingStore(e).UpdateActivity(meetingCtx(e), ahead, UpdateActivityInput{MeetingStatus: &canceled})
	if err != nil {
		t.Fatalf("canceling tomorrow's meeting: %v", err)
	}
	if out.MeetingStatus == nil || string(*out.MeetingStatus) != canceled {
		t.Errorf("the meeting reads %v after canceling, want canceled", out.MeetingStatus)
	}
}

// The start the patch leaves the meeting with is the one judged. Moving a
// meeting into the past and saying it was held is one patch.
func TestAMeetingMovedIntoThePastCanBeHeldInOnePatch(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	booked := "booked"
	ahead := customerMeeting(t, e, customer, "Booked for tomorrow", clocktest.Now(t).AddDate(0, 0, 1), &booked)

	held := string(crmcontracts.ActivityMeetingStatusHeld)
	out, err := meetingStore(e).UpdateActivity(meetingCtx(e), ahead, UpdateActivityInput{
		MeetingStatus: &held, OccurredAt: new(clocktest.Now(t).Add(-time.Hour)),
	})
	if err != nil {
		t.Fatalf("holding a meeting moved into the past: %v", err)
	}
	if out.AwaitingOutcome == nil || *out.AwaitingOutcome {
		t.Errorf("the held meeting reads awaiting_outcome %v, want false", out.AwaitingOutcome)
	}
}
