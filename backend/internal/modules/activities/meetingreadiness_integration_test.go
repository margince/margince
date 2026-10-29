// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// An attendee's "I'm ready", written through ConfirmMeetingReady and read back
// through MeetingPreparations, the read the Worklist and prep_for_meeting use.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// seatCtx binds a rep-like seat other than e.rep.
func (e *sendEnv) seatCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// sharedMeeting logs a meeting the rep hosts, with the colleague invited.
func sharedMeeting(t *testing.T, e *sendEnv, start time.Time) ids.ActivityID {
	t.Helper()
	subject, booked := "Pricing review", string(crmcontracts.ActivityMeetingStatusBooked)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start,
		MeetingStatus: &booked, Source: "manual", AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.other)},
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(out.Id))
}

func preparationOf(t *testing.T, e *sendEnv, ctx context.Context, meeting ids.ActivityID) (MeetingPreparation, bool) {
	t.Helper()
	read, err := meetingStore(e).MeetingPreparations(ctx, []ids.UUID{meeting.UUID})
	if err != nil {
		t.Fatalf("reading the preparation: %v", err)
	}
	got, ok := read[meeting.UUID]
	return got, ok
}

func reschedule(t *testing.T, e *sendEnv, meeting ids.ActivityID, start time.Time) {
	t.Helper()
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), meeting, UpdateActivityInput{OccurredAt: &start}); err != nil {
		t.Fatalf("moving the meeting: %v", err)
	}
}

// readinessTrail counts the audit rows and events one meeting's readiness left,
// and names the actor type of the last audit row.
func readinessTrail(t *testing.T, e *sendEnv, meeting ids.ActivityID) (audited, announced int, lastActor string) {
	t.Helper()
	ctx := context.Background()
	if err := e.owner.QueryRow(ctx, `
		SELECT count(*), COALESCE((array_agg(actor_type ORDER BY id DESC))[1], '')
		  FROM audit_log WHERE entity_id = $1 AND after ? 'readiness'`, meeting.UUID).Scan(&audited, &lastActor); err != nil {
		t.Fatalf("counting the audit rows: %v", err)
	}
	if err := e.owner.QueryRow(ctx, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'activity.readiness_recorded' AND envelope->'entity'->>'id' = $1`,
		meeting.UUID.String()).Scan(&announced); err != nil {
		t.Fatalf("counting the events: %v", err)
	}
	return audited, announced, lastActor
}

func TestAnAttendeeConfirmsTheyAreReadyAtTheRevisionTheyRead(t *testing.T) {
	e := setupSend(t)
	meeting := sharedMeeting(t, e, clocktest.Now(t).Add(time.Hour))
	rep := meetingCtx(e)
	before, ok := preparationOf(t, e, rep, meeting)
	if !ok || before.Ready || before.Revision == "" {
		t.Fatalf("before any confirmation the host reads %+v (present %v), want a revision that is not ready", before, ok)
	}

	after, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, before.Revision)
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if !after.Ready || after.Revision != before.Revision {
		t.Errorf("the write answered %+v, want ready at %s", after, before.Revision)
	}
	if read, _ := preparationOf(t, e, rep, meeting); !read.Ready {
		t.Error("the confirmation did not read back as ready")
	}
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, before.Revision); err != nil {
		t.Fatalf("confirming twice: %v", err)
	}
	if audited, announced, actor := readinessTrail(t, e, meeting); audited != 1 || announced != 1 || actor != "human" {
		t.Errorf("two presses left %d audit rows and %d events by %q, want one of each by the human", audited, announced, actor)
	}
}

func TestAStaleRevisionIsRefusedAndTheMovedStartIsNamed(t *testing.T) {
	e := setupSend(t)
	start := clocktest.Now(t).Add(time.Hour)
	meeting := sharedMeeting(t, e, start)
	rep := meetingCtx(e)
	first, _ := preparationOf(t, e, rep, meeting)
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, first.Revision); err != nil {
		t.Fatalf("confirming: %v", err)
	}

	reschedule(t, e, meeting, start.Add(30*time.Minute))

	moved, _ := preparationOf(t, e, rep, meeting)
	if moved.Ready || len(moved.Changed) != 1 || moved.Changed[0] != FactStartTime {
		t.Errorf("after the move the host reads %+v, want needs review naming only the start time", moved)
	}
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, first.Revision); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("confirming the old revision answered %v, want version skew", err)
	}
	if again, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, moved.Revision); err != nil || !again.Ready {
		t.Errorf("confirming the current revision answered %+v, %v; want ready", again, err)
	}
}

func TestUndoTakesBackOnlyTheConfirmationItWasFor(t *testing.T) {
	e := setupSend(t)
	start := clocktest.Now(t).Add(time.Hour)
	meeting := sharedMeeting(t, e, start)
	rep := meetingCtx(e)
	old, _ := preparationOf(t, e, rep, meeting)
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, old.Revision); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	reschedule(t, e, meeting, start.Add(time.Hour))
	current, _ := preparationOf(t, e, rep, meeting)
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, current.Revision); err != nil {
		t.Fatalf("confirming the new revision: %v", err)
	}

	stale, err := meetingStore(e).WithdrawMeetingReady(rep, meeting, old.Revision)
	if err != nil {
		t.Fatalf("undoing the old press: %v", err)
	}
	if !stale.Ready {
		t.Error("undoing the old press took back the newer confirmation")
	}
	undone, err := meetingStore(e).WithdrawMeetingReady(rep, meeting, current.Revision)
	if err != nil || undone.Ready {
		t.Errorf("undoing the current press answered %+v, %v; want not ready", undone, err)
	}
}

func TestOneAttendeesReadinessLeavesTheOtherUnprepared(t *testing.T) {
	e := setupSend(t)
	meeting := sharedMeeting(t, e, clocktest.Now(t).Add(time.Hour))
	rep, colleague := meetingCtx(e), e.seatCtx(e.other)
	read, _ := preparationOf(t, e, rep, meeting)
	if _, err := meetingStore(e).ConfirmMeetingReady(rep, meeting, read.Revision); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	theirs, ok := preparationOf(t, e, colleague, meeting)
	if !ok || theirs.Ready || theirs.Revision != read.Revision {
		t.Errorf("the invited colleague reads %+v (present %v), want the same revision, not ready", theirs, ok)
	}
}

func TestOnlyAnAttendeeCanSayTheyAreReady(t *testing.T) {
	e := setupSend(t)
	meeting := bookMeeting(t, e, "Rep alone", clocktest.Now(t).Add(time.Hour))
	read, _ := preparationOf(t, e, meetingCtx(e), meeting)
	outsider := e.seatCtx(e.other)
	if _, ok := preparationOf(t, e, outsider, meeting); ok {
		t.Error("a colleague who does not attend the meeting read a preparation state for it")
	}
	_, err := meetingStore(e).ConfirmMeetingReady(outsider, meeting, read.Revision)
	var refusal *values.ParseError
	if !errors.As(err, &refusal) || refusal.Code != "not_an_attendee" {
		t.Errorf("a non-attendee's confirmation answered %v, want not_an_attendee", err)
	}
}

func TestAnAgentConfirmsForItsUserAndTheTrailNamesTheAgent(t *testing.T) {
	e := setupSend(t)
	meeting := sharedMeeting(t, e, clocktest.Now(t).Add(time.Hour))
	read, _ := preparationOf(t, e, meetingCtx(e), meeting)
	human, _ := principal.Actor(meetingCtx(e))
	agent := principal.WithActor(meetingCtx(e), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		UserID: e.rep, OnBehalfOf: e.rep, PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite), Permissions: human.Permissions,
	})

	if _, err := meetingStore(e).ConfirmMeetingReady(agent, meeting, read.Revision); err != nil {
		t.Fatalf("the agent confirming: %v", err)
	}
	if mine, _ := preparationOf(t, e, meetingCtx(e), meeting); !mine.Ready {
		t.Error("the agent's confirmation did not make its user ready")
	}
	if _, _, actor := readinessTrail(t, e, meeting); actor != string(principal.PrincipalAgent) {
		t.Errorf("the audit row names a %q actor, want the agent", actor)
	}
}
