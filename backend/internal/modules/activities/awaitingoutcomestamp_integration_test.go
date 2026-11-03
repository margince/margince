// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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

// A patch that moves a held meeting to a start still ahead is refused. It sends
// no status, and the status it leaves is the one judged.
func TestMovingAHeldMeetingAheadIsRefused(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	noon := clocktest.Now(t).UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	store := meetingStore(e).WithClock(func() time.Time { return noon }).
		WithRecordZone(func(context.Context, pgx.Tx) (*time.Location, error) { return time.UTC, nil })
	held, subject := string(crmcontracts.ActivityMeetingStatusHeld), "Held an hour ago"
	for name, patch := range map[string]UpdateActivityInput{
		"a start tomorrow":    {OccurredAt: new(noon.AddDate(0, 0, 1))},
		"today, with no time": {OccurredPrecision: new(PrecisionDay)},
	} {
		met, _, err := store.LogActivity(meetingCtx(e), LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: new(noon.Add(-time.Hour)),
			MeetingStatus: &held, Source: "manual", Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
		})
		if err != nil {
			t.Fatalf("logging the held meeting: %v", err)
		}
		_, err = store.UpdateActivity(meetingCtx(e), ids.From[ids.ActivityKind](ids.UUID(met.Id)), patch)
		if _, early := errors.AsType[*MeetingNotTakenPlaceError](err); !early {
			t.Errorf("moving a held meeting to %s answered %v, want the not-taken-place refusal", name, err)
		}
	}
}

// A form resending the start a held meeting already has moves nothing. So a
// row stored held ahead of its start still takes an edit to its subject.
func TestResendingAHeldMeetingsStartIsNoMove(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	booked := "booked"
	start := clocktest.Now(t).UTC().Truncate(time.Second).AddDate(0, 0, 7)
	ahead := customerMeeting(t, e, customer, "Held ahead of its start", start, &booked)
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET meeting_status = 'held' WHERE id = $1`, ahead.UUID); err != nil {
		t.Fatalf("storing the meeting held ahead of its start: %v", err)
	}

	subject := "Renamed"
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), ahead, UpdateActivityInput{Subject: &subject, OccurredAt: &start}); err != nil {
		t.Errorf("resending the unchanged start with a new subject answered %v, want it accepted", err)
	}
}

// A date-only meeting takes place once its date is over in the installation's
// timezone. Dated today there, it is not owed yet and cannot be held.
func TestADateOnlyMeetingIsOwedOnceItsLocalDateIsOver(t *testing.T) {
	e := setupSend(t)
	setInstallationZone(t, e, "Pacific/Auckland")
	zone, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	// Two days behind the wall clock, so a stamp read at wall time owes "today" too.
	now := clocktest.Now(t).UTC().Truncate(24*time.Hour).Add(22*time.Hour).AddDate(0, 0, -2)
	store := meetingStore(e).WithClock(func() time.Time { return now })
	customer := seedCustomer(t, e)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	booked, held := "booked", string(crmcontracts.ActivityMeetingStatusHeld)
	for _, c := range []struct {
		subject string
		date    time.Time
		owed    bool
	}{
		{"Dated today in Auckland", today, false},
		{"Dated yesterday in Auckland", today.AddDate(0, 0, -1), true},
	} {
		logged, _, err := store.LogActivity(meetingCtx(e), LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &c.subject, OccurredAt: &c.date,
			OccurredPrecision: PrecisionDay, MeetingStatus: &booked, Source: "manual",
			Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", c.subject, err)
		}
		id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
		read, err := store.GetActivity(meetingCtx(e), id, storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading %q: %v", c.subject, err)
		}
		if read.AwaitingOutcome == nil || *read.AwaitingOutcome != c.owed {
			t.Errorf("%q reads awaiting_outcome %v, want %v", c.subject, read.AwaitingOutcome, c.owed)
		}
		unchanged, err := store.UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Subject: &c.subject})
		if err != nil {
			t.Fatalf("resending the subject of %q: %v", c.subject, err)
		}
		if unchanged.AwaitingOutcome == nil || *unchanged.AwaitingOutcome != c.owed {
			t.Errorf("%q answers a patch that changes nothing with awaiting_outcome %v, want %v",
				c.subject, unchanged.AwaitingOutcome, c.owed)
		}
		_, err = store.UpdateActivity(meetingCtx(e), id, UpdateActivityInput{MeetingStatus: &held})
		var early *MeetingNotTakenPlaceError
		switch {
		case c.owed && err != nil:
			t.Errorf("holding %q answered %v, want it accepted", c.subject, err)
		case !c.owed && !errors.As(err, &early):
			t.Errorf("holding %q answered %v, want the not-taken-place refusal", c.subject, err)
		}
	}
}

// Logging a meeting is held to the same rule as a patch. So is a second log
// under the same source key that moves the status of the row it holds.
func TestLoggingAnOutcomeWaitsForTheMeetingToStart(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	tomorrow := clocktest.Now(t).AddDate(0, 0, 1)
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	logged := func(status, sourceID string) error {
		subject, system := "Booked for tomorrow", "crm-sync"
		_, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &tomorrow,
			MeetingStatus: &status, Source: "manual", SourceSystem: &system, SourceID: &sourceID,
			Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
		})
		return err
	}
	var early *MeetingNotTakenPlaceError
	if err := logged(held, "fresh"); !errors.As(err, &early) {
		t.Fatalf("logging tomorrow's meeting as held answered %v, want the not-taken-place refusal", err)
	}
	if err := logged("booked", "replayed"); err != nil {
		t.Fatalf("logging tomorrow's meeting as booked: %v", err)
	}
	if err := logged(held, "replayed"); !errors.As(err, &early) {
		t.Errorf("replaying it as held answered %v, want the not-taken-place refusal", err)
	}
}
