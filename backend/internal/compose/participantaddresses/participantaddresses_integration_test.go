// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package participantaddresses

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func systemCtx(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:datatask-test")
}

// older plants meetings and the guest rows a writer stored before the table
// held addresses lowercase.
type older struct {
	t     *testing.T
	e     *integration.Env
	owner *pgx.Conn
}

func seedOlder(t *testing.T) older {
	t.Helper()
	return older{t: t, e: integration.Setup(t), owner: integration.OwnerConn(t)}
}

func (o older) meeting() ids.UUID {
	o.t.Helper()
	id := ids.NewV7()
	if _, err := o.owner.Exec(context.Background(), `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Fleet renewal', now(), 'manual', 'human:x')`, id); err != nil {
		o.t.Fatalf("seeding a meeting: %v", err)
	}
	return id
}

// guest is one guest row. A seat is stored in the older single roster column.
type guest struct {
	address  string
	response *string
	name     *string
	seat     *ids.UUID
	written  time.Time
}

// asTyped plants g with its address as given, which today's writers refuse.
func (o older) asTyped(meeting ids.UUID, g guest) ids.UUID {
	o.t.Helper()
	id := ids.NewV7()
	testdb.WithParticipantAddressCheckLifted(o.t, func(tx pgx.Tx) error {
		_, err := tx.Exec(o.t.Context(), `
			INSERT INTO activity_participant (id, activity_id, role, address, response, display_name, roster_seat, created_at)
			VALUES ($1, $2, 'attendee', $3, $4, $5, $6, $7)`, id, meeting, g.address, g.response, g.name, g.seat, o.at(g))
		return err
	})
	return id
}

// lowercase plants g as today's writers store it, in the seat set.
func (o older) lowercase(meeting ids.UUID, g guest) {
	o.t.Helper()
	var seats []ids.UUID
	if g.seat != nil {
		seats = []ids.UUID{*g.seat}
	}
	o.e.WsExec(o.t, `INSERT INTO activity_participant (activity_id, role, address, response, display_name, roster_seats, created_at)
		VALUES ($1, 'attendee', $2, $3, $4, $5, $6)`, meeting, g.address, g.response, g.name, seats, o.at(g))
}

func (o older) at(g guest) time.Time {
	if g.written.IsZero() {
		return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	}
	return g.written
}

// stored is what one address reads as on a meeting after the correction. It
// counts the rows and the seats in each of the two roster columns.
type stored struct {
	Rows              int
	Response, Name    *string
	Seats, OlderSeats int
}

func (o older) stored(meeting ids.UUID, address string) stored {
	o.t.Helper()
	var s stored
	if err := o.owner.QueryRow(context.Background(), `
		SELECT count(*), min(response), min(display_name), coalesce(sum(cardinality(roster_seats)), 0), count(roster_seat)
		  FROM activity_participant WHERE activity_id = $1 AND address = $2`, meeting, address).
		Scan(&s.Rows, &s.Response, &s.Name, &s.Seats, &s.OlderSeats); err != nil {
		o.t.Fatalf("reading the rows of %s: %v", address, err)
	}
	return s
}

func read(s *string) string {
	if s == nil {
		return "<null>"
	}
	return *s
}

// Two spellings of one address on one meeting and role become one row. It keeps
// the given answer, the name and the seats of both calendars.
func TestTwoSpellingsOfOneGuestFoldIntoOneRowThatKeepsWhatBothKnew(t *testing.T) {
	o := seedOlder(t)
	meeting := o.meeting()
	o.asTyped(meeting, guest{address: " Lena@Prospect.test", name: new("Lena Prospect"), seat: &o.e.Rep1})
	o.lowercase(meeting, guest{address: "lena@prospect.test", response: new("accepted"), name: new(""), seat: &o.e.Rep2})
	o.lowercase(meeting, guest{address: "omar@prospect.test", response: new("declined")})
	o.e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'organizer', 'lena@prospect.test')`, meeting)

	ctx := systemCtx(o.e)
	step := DataTask().Steps[0]
	if n, capped, err := step.Estimate(ctx, o.e.Pool, 10); err != nil || n != 1 || capped {
		t.Fatalf("the estimate = %d, capped %v (%v), want the one row as typed", n, capped, err)
	}
	o.e.RunDataTask(ctx, t, DataTask())

	got := o.stored(meeting, "lena@prospect.test")
	if got.Rows != 2 {
		t.Fatalf("%d row(s) name the guest, want the attendee row and the organizer row", got.Rows)
	}
	attendee := o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role = 'attendee'
		AND address = 'lena@prospect.test' AND response = 'accepted' AND display_name = 'Lena Prospect'
		AND roster_seats = $2 AND roster_seat IS NULL`, meeting, sortedSeats(o.e.Rep1, o.e.Rep2))
	if attendee != 1 {
		t.Errorf("%d attendee row(s) keep the answer, the name and both seats, want 1", attendee)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1`, meeting); n != 3 {
		t.Errorf("the meeting holds %d participant row(s), want 3", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'
		AND before->>'participant_addresses_not_lowercase' = '1' AND after->>'participant_addresses_not_lowercase' = '0'
		AND evidence ? 'data_task_run'`, meeting); n != 1 {
		t.Errorf("%d audit row(s) record the correction on the meeting, want 1", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o
		JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = $1 AND o.envelope->>'type' = 'activity.updated'`, meeting); n != 1 {
		t.Errorf("%d event(s) announce the meeting's changed guests, want 1", n)
	}
	if n, _, err := step.Estimate(ctx, o.e.Pool, 10); err != nil || n != 0 {
		t.Errorf("the step still estimates %d row(s) (%v), want none", n, err)
	}
}

func sortedSeats(a, b ids.UUID) []ids.UUID {
	if a.String() > b.String() {
		return []ids.UUID{b, a}
	}
	return []ids.UUID{a, b}
}

// Between two given answers the later written row wins and brings its name. An
// answer beats an invitation not answered yet, whichever row is later.
func TestTheLaterGivenAnswerWinsAndBringsItsName(t *testing.T) {
	o := seedOlder(t)
	early, late := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		typed, lowered guest
		response, name string
	}{
		"the row as typed is later": {
			typed:    guest{address: "Ada@Buyer.test", response: new("declined"), name: new("Ada, later"), written: late},
			lowered:  guest{address: "ada@buyer.test", response: new("accepted"), name: new("Ada, earlier"), written: early},
			response: "declined", name: "Ada, later",
		},
		"the lowercase row is later": {
			typed:    guest{address: "Ben@Buyer.test", response: new("declined"), name: new("Ben, earlier"), written: early},
			lowered:  guest{address: "ben@buyer.test", response: new("tentative"), name: new("Ben, later"), written: late},
			response: "tentative", name: "Ben, later",
		},
		"an earlier answer beats a later open invitation": {
			typed:    guest{address: "Cy@Buyer.test", response: new("accepted"), written: early},
			lowered:  guest{address: "cy@buyer.test", response: new("needs_action"), name: new("Cy"), written: late},
			response: "accepted", name: "Cy",
		},
		"no answer on either row keeps the open invitation": {
			typed:    guest{address: "Di@Buyer.test", written: late},
			lowered:  guest{address: "di@buyer.test", response: new("needs_action"), written: early},
			response: "needs_action", name: "<null>",
		},
	}
	meetings := map[string]ids.UUID{}
	for name, c := range cases {
		meetings[name] = o.meeting()
		o.asTyped(meetings[name], c.typed)
		o.lowercase(meetings[name], c.lowered)
	}
	o.e.RunDataTask(systemCtx(o.e), t, DataTask())
	for name, c := range cases {
		got := o.stored(meetings[name], c.lowered.address)
		if got.Rows != 1 || read(got.Response) != c.response || read(got.Name) != c.name {
			t.Errorf("%s: %d row(s) with answer %s and name %s, want 1 with %s and %s",
				name, got.Rows, read(got.Response), read(got.Name), c.response, c.name)
		}
	}
}

// Batches of two walk the rows in key order until each address is lowercase.
// Three spellings of one guest end as one row, and a replay changes nothing.
func TestBatchesOfTwoReachEveryAddressAndAReplayChangesNothing(t *testing.T) {
	o := seedOlder(t)
	crowded := o.meeting()
	for _, address := range []string{"Eve@Buyer.test", "eve@BUYER.test", "EVE@buyer.test"} {
		o.asTyped(crowded, guest{address: address})
	}
	for _, address := range []string{"Fay@Buyer.test", "Gus@Buyer.test"} {
		o.asTyped(o.meeting(), guest{address: address})
	}
	ctx := systemCtx(o.e)
	lowercase := DataTask().Steps[0].Batch
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for batches := 1; ; batches++ {
		var out datatask.BatchOut
		if err := database.WithTx(ctx, o.e.Pool, func(tx pgx.Tx) (err error) {
			out, err = lowercase(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatalf("batch %d: %v", batches, err)
		}
		if out.Changed > 2 {
			t.Fatalf("batch %d changed %d rows, over its limit of 2", batches, out.Changed)
		}
		if out.Done {
			break
		}
		if batches > 5 {
			t.Fatal("the walk did not finish in the batches five rows need")
		}
		in.Cursor = out.Cursor
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE address <> lower(btrim(address))`); n != 0 {
		t.Errorf("%d address(es) were never reached", n)
	}
	if got := o.stored(crowded, "eve@buyer.test"); got.Rows != 1 {
		t.Errorf("three spellings of one guest left %d row(s), want 1", got.Rows)
	}
	if err := database.WithTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		out, err := lowercase(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || !out.Done) {
			t.Errorf("a replay changed %d row(s), done %v, want none and done", out.Changed, out.Done)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
}

// A batch that fails after its fold leaves the rows and the audit log as they
// were. A cursor that is no key is refused, and so is a caller that is no system.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	o := seedOlder(t)
	meeting := o.meeting()
	o.asTyped(meeting, guest{address: "Hal@Buyer.test"})
	o.lowercase(meeting, guest{address: "hal@buyer.test", response: new("accepted")})
	ctx := systemCtx(o.e)
	task := DataTask()
	lowercase := task.Steps[0].Batch
	interrupted := false
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := lowercase(ctx, tx, in)
		if err != nil {
			return out, err
		}
		interrupted = out.Changed == 1
		return out, errors.New("interrupt after the fold")
	}
	o.e.RunDataTask(ctx, t, task)
	if !interrupted {
		t.Fatal("the batch failed before its fold, so no rollback was tested")
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1`, meeting); n != 2 {
		t.Errorf("the rolled-back batch left %d row(s), want both spellings", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, meeting); n != 0 {
		t.Errorf("the rolled-back batch left %d audit row(s)", n)
	}
	if err := database.WithTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		_, err := lowercase(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
	// A batch or an estimate whose context ended reports the failed read.
	stopped, stop := context.WithCancel(ctx)
	stop()
	if err := database.WithTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		_, err := lowercase(stopped, tx, datatask.BatchIn{Limit: 2})
		return err
	}); err == nil {
		t.Fatal("a batch whose context ended reported success")
	}
	if _, _, err := DataTask().Steps[0].Estimate(stopped, o.e.Pool, 10); err == nil {
		t.Fatal("an estimate whose context ended reported a count")
	}
	member := o.e.Admin()
	if err := database.WithTx(member, o.e.Pool, func(tx pgx.Tx) error {
		_, err := lowercase(member, tx, datatask.BatchIn{Limit: 2})
		return err
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member running the correction: err = %v, want permission denied", err)
	}
}

// An unvalidated check still judges an older row when it is next written. An
// edit before the task has run goes through, and leaves the address lowercase.
func TestAnEditBeforeTheTaskLowercasesAnOlderRow(t *testing.T) {
	o := seedOlder(t)
	alone, twinned := o.meeting(), o.meeting()
	single := o.asTyped(alone, guest{address: "Ivy@Buyer.test"})
	typed := o.asTyped(twinned, guest{address: "Jon@Buyer.test", name: new("Jon"), seat: &o.e.Rep1})
	o.lowercase(twinned, guest{address: "jon@buyer.test", response: new("declined"), seat: &o.e.Rep2})

	if err := o.e.WsExecErr(t, `UPDATE activity_participant SET display_name = 'Ivy' WHERE id = $1`, single); err != nil {
		t.Fatalf("editing an older row: %v", err)
	}
	if got := o.stored(alone, "ivy@buyer.test"); got.Rows != 1 || read(got.Name) != "Ivy" {
		t.Errorf("the edited row reads %d row(s) named %s under its lowercase address, want 1 named Ivy", got.Rows, read(got.Name))
	}
	if err := o.e.WsExecErr(t, `UPDATE activity_participant SET response = 'accepted' WHERE id = $1`, typed); err != nil {
		t.Fatalf("answering on an older row that has a lowercase twin: %v", err)
	}
	got := o.stored(twinned, "jon@buyer.test")
	if got.Rows != 1 || read(got.Response) != "accepted" || read(got.Name) != "Jon" || got.Seats != 2 || got.OlderSeats != 0 {
		t.Errorf("the answered row reads %+v, want one row with the answer just given, the name and both seats", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE id = $1`, typed); n != 1 {
		t.Errorf("the row that was edited is gone; the fold keeps it and removes its twin")
	}
	spelledTwice := o.meeting()
	o.asTyped(spelledTwice, guest{address: "Lia@Buyer.test"})
	o.asTyped(spelledTwice, guest{address: "LIA@Buyer.test"})
	if err := o.e.WsExecErr(t, `UPDATE activity_participant SET display_name = 'Lia' WHERE activity_id = $1`, spelledTwice); err != nil {
		t.Fatalf("one statement editing two older spellings of a guest: %v", err)
	}
	if got := o.stored(spelledTwice, "lia@buyer.test"); got.Rows != 1 || read(got.Name) != "Lia" {
		t.Errorf("two older spellings edited together read %+v, want one row named Lia", got)
	}
	err := o.e.WsExecErr(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', 'Kim@Buyer.test')`, alone)
	if err == nil {
		t.Fatal("a new row stored an address that is not lowercase")
	}
}
