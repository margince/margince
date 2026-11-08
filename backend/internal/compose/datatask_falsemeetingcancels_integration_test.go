// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// falseCancelClock is the repair's today: every fixture meeting starts in
// January 2030, and one 40 days before is outside its window.
func falseCancelTask() datatask.Task {
	return falseMeetingCancelsDataTask(func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) })
}

func runFalseCancelTask(t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
}

// cancelAs runs the calendar cancellation older code sent for every removed or
// declined copy, as the seat whose calendar synced it.
func (o ownershipEnv) cancelAs(t *testing.T, seat ids.UUID, c mailboxCopy) {
	t.Helper()
	c.provider, c.owner = o.provider, o.address[seat]
	if c.uid == "" {
		c.uid = sharedMeetingUID
	}
	rec := c.record(t)
	if err := o.sink.CancelIdentifiedMeeting(connectorOwnerCtx(o.e, seat, o.provider), rec.NaturalKey, rec.CrossDoorIdentity, c.start()); err != nil {
		t.Fatal(err)
	}
}

func meetingStatusOf(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	status, _ := readMeetingStatus(t, e, id)
	return status
}

func TestTheRepairReopensOnlyAMeetingAGuestsCalendarCancelled(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			o := setupOwnership(t, provider)
			e := o.e
			organizer, guest := o.address[e.Rep1], o.address[e.Rep2]
			guests := map[string]string{guest: "accepted", "buyer@customer.test": "accepted"}
			copyOf := func(id, uid string, day int) mailboxCopy {
				return mailboxCopy{id: id, uid: uid, organizer: organizer, guests: guests, day: day}
			}
			reopened := o.sync(t, e.Rep1, copyOf("evt-1", "guest-cancelled@calendar.test", 0))
			o.sync(t, e.Rep2, copyOf("evt-2", "guest-cancelled@calendar.test", 0))
			o.cancelAs(t, e.Rep2, copyOf("evt-2", "guest-cancelled@calendar.test", 0))
			kept := o.sync(t, e.Rep1, copyOf("evt-3", "organizer-cancelled@calendar.test", 1))
			o.cancelAs(t, e.Rep1, copyOf("evt-3", "organizer-cancelled@calendar.test", 1))
			old := o.sync(t, e.Rep1, copyOf("evt-4", "long-past@calendar.test", -40))
			o.sync(t, e.Rep2, copyOf("evt-5", "long-past@calendar.test", -40))
			o.cancelAs(t, e.Rep2, copyOf("evt-5", "long-past@calendar.test", -40))
			for _, id := range []ids.UUID{reopened, kept, old} {
				if got := meetingStatusOf(t, e, id); got != "canceled" {
					t.Fatalf("fixture meeting reads %q before the repair, want canceled", got)
				}
			}

			runFalseCancelTask(t, e, falseCancelTask())

			if status, set := readMeetingStatus(t, e, reopened); set {
				t.Errorf("the meeting a guest's calendar cancelled reads %s, want its unanswered status back", status)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity_meeting_history WHERE activity_id=$1 AND status='booked'
 AND source_system=$2 AND source_id='evt-2'`, reopened, provider); n != 1 {
				t.Errorf("the reopen recorded %d transitions naming the cancelled event, want 1", n)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND evidence->>'reopened'=$2`, reopened, activities.CalendarCancellationReopened); n != 1 {
				t.Errorf("the reopen wrote %d audit rows naming the repair, want 1", n)
			}
			if got := meetingStatusOf(t, e, kept); got != "canceled" {
				t.Errorf("the organizer's cancellation reads %q, want canceled", got)
			}
			if got := meetingStatusOf(t, e, old); got != "canceled" {
				t.Errorf("a meeting older than the window reads %q, want canceled", got)
			}
		})
	}
}

func TestTheRepairKeepsAStatusAHumanSetAfterTheCalendar(t *testing.T) {
	o := setupOwnership(t, sourceGCal)
	e := o.e
	guests := map[string]string{o.address[e.Rep2]: "accepted", "buyer@customer.test": "accepted"}
	meeting := o.sync(t, e.Rep1, mailboxCopy{id: "evt-1", organizer: o.address[e.Rep1], guests: guests})
	o.sync(t, e.Rep2, mailboxCopy{id: "evt-2", organizer: o.address[e.Rep1], guests: guests})
	o.cancelAs(t, e.Rep2, mailboxCopy{id: "evt-2", organizer: o.address[e.Rep1], guests: guests})
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	for _, status := range []string{"booked", "canceled"} {
		if _, err := activities.NewStore(e.DB()).UpdateActivity(human, ids.From[ids.ActivityKind](meeting),
			activities.UpdateActivityInput{MeetingStatus: &status}); err != nil {
			t.Fatal(err)
		}
	}

	runFalseCancelTask(t, e, falseCancelTask())

	if got := meetingStatusOf(t, e, meeting); got != "canceled" {
		t.Fatalf("a cancellation a human recorded reads %q after the repair, want canceled", got)
	}
}

// closedByTheBackfill is the old stored-original backfill's verdict on a
// meeting: closed by the shared cancel writer, and marked closed.
func closedByTheBackfill(t *testing.T, e *integration.Env, meeting ids.UUID, eventID string) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:participant_backfill")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		key := connector.NaturalKey{SourceSystem: sourceGCal, SourceID: eventID}
		if _, closed, err := activities.CancelCapturedMeetingTx(ctx, tx, key, time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)); err != nil || !closed {
			return errors.Join(err, errors.New("the backfill's cancel closed nothing"))
		}
		return markMeetingRSVPSettled(ctx, tx, ids.From[ids.ActivityKind](meeting), rsvpBackfillClosed)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheRepairReopensABackfillClosureOnlyWhereTheOriginalNoLongerCancels(t *testing.T) {
	o := setupOwnership(t, sourceGCal)
	e := o.e
	owner := o.address[e.Rep1]
	guests := map[string]string{owner: "accepted", "buyer@customer.test": "accepted"}
	reopened := o.sync(t, e.Rep1, mailboxCopy{id: "evt-on", uid: "on@calendar.test", organizer: "host@customer.test", guests: guests})
	closedByTheBackfill(t, e, reopened, "evt-on")

	// The original the first delivery stored said the organizer called it off.
	cancelled, err := json.Marshal(map[string]any{
		"id": "evt-off", "iCalUID": "off@calendar.test", "status": "cancelled",
		"organizer": map[string]any{"email": owner, "self": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', 'evt-off', $1)`, cancelled); err != nil {
		t.Fatal(err)
	}
	kept := o.sync(t, e.Rep1, mailboxCopy{id: "evt-off", uid: "off@calendar.test", organizer: owner, guests: guests, day: 1})
	closedByTheBackfill(t, e, kept, "evt-off")

	runFalseCancelTask(t, e, falseCancelTask())

	if status, set := readMeetingStatus(t, e, reopened); set {
		t.Errorf("a closure today's rules undo reads %s after the repair", status)
	}
	if got := meetingStatusOf(t, e, kept); got != "canceled" {
		t.Errorf("a closure the original still supports reads %q, want canceled", got)
	}
}

func TestTheRepairPagesReplaysAndRollsBackWithoutChangingAnythingTwice(t *testing.T) {
	o := setupOwnership(t, sourceGraphCal)
	e := o.e
	guests := map[string]string{o.address[e.Rep2]: "accepted", "buyer@customer.test": "accepted"}
	var meetings []ids.UUID
	for i, uid := range []string{"first@calendar.test", "second@calendar.test"} {
		own, guest := "evt-own-"+uid, "evt-guest-"+uid
		meetings = append(meetings, o.sync(t, e.Rep1, mailboxCopy{id: own, uid: uid, organizer: o.address[e.Rep1], guests: guests, day: i}))
		o.sync(t, e.Rep2, mailboxCopy{id: guest, uid: uid, organizer: o.address[e.Rep1], guests: guests, day: i})
		o.cancelAs(t, e.Rep2, mailboxCopy{id: guest, uid: uid, organizer: o.address[e.Rep1], guests: guests, day: i})
	}

	interrupted := falseCancelTask()
	original := interrupted.Steps[0].Batch
	interrupted.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runFalseCancelTask(t, e, interrupted)
	for _, id := range meetings {
		if got := meetingStatusOf(t, e, id); got != "canceled" {
			t.Fatalf("an interrupted batch left a meeting %q, want its reopen rolled back", got)
		}
	}

	if changed := walkFalseCancelTask(t, e, nil); changed != 2 {
		t.Fatalf("a walk one meeting at a time reopened %d meetings, want 2", changed)
	}
	if changed := walkFalseCancelTask(t, e, nil); changed != 0 {
		t.Fatalf("a replay from the start reopened %d meetings, want none", changed)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_meeting_history WHERE activity_id = ANY($1) AND status='booked'`, meetings); n != 2 {
		t.Fatalf("the meetings hold %d reopen transitions, want one each", n)
	}

	ctx := datatask.Acting(e.Admin(), falseCancelTask().ID)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := falseCancelTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"not an id"`), Limit: 10})
		return err
	})
	if err == nil {
		t.Fatal("a batch accepted a cursor that names no meeting")
	}
}

// walkFalseCancelTask runs the step one meeting per batch from cursor and
// answers how many meetings it reopened.
func walkFalseCancelTask(t *testing.T, e *integration.Env, cursor json.RawMessage) int {
	t.Helper()
	task := falseCancelTask()
	ctx := datatask.Acting(e.Admin(), task.ID)
	in := datatask.BatchIn{Cursor: cursor, Limit: 1, RunID: ids.NewV7()}
	changed := 0
	for range 20 {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = task.Steps[0].Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		changed += out.Changed
		if out.Done {
			return changed
		}
		in.Cursor = out.Cursor
	}
	t.Fatal("the walk never finished")
	return changed
}
