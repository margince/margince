// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The noon repair and the precision task, in both upgrade orders. Meetings are
// damaged through the same outcome save the noon tests use.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// precisions lists the recorded precision of each of a meeting's history
// rows, oldest first, with "unknown" where none was recorded.
func (f noonFixture) precisions(id ids.ActivityID) []string {
	f.t.Helper()
	rows, err := integration.OwnerConn(f.t).Query(context.Background(), `SELECT coalesce(scheduled_precision, 'unknown')
		FROM activity_meeting_history WHERE activity_id = $1 ORDER BY effective_at, id`, id.UUID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var precision string
		if err := rows.Scan(&precision); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, precision)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

// forgetMeetingPrecision makes the meetings' history rows look as they did
// before the precision column existed. The column's rule binds only new rows,
// so the edit drops it and adds it back unvalidated, as the migration left it.
func forgetMeetingPrecision(t *testing.T, conn *pgx.Conn, meetings ...ids.ActivityID) {
	t.Helper()
	keys := make([]ids.UUID, 0, len(meetings))
	for _, meeting := range meetings {
		keys = append(keys, meeting.UUID)
	}
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE activity_meeting_history DROP CONSTRAINT activity_meeting_history_start_precision`, nil},
		{`UPDATE activity_meeting_history SET scheduled_precision = NULL WHERE activity_id = ANY($1)`, []any{keys}},
		{`ALTER TABLE activity_meeting_history ADD CONSTRAINT activity_meeting_history_start_precision
		   CHECK (CASE WHEN scheduled_start IS NULL THEN scheduled_precision IS NULL
		               ELSE scheduled_precision IN ('instant', 'day') END) NOT VALID`, nil},
	} {
		if _, err := conn.Exec(context.Background(), step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
}

// Upgraded before the repair ran: the repair itself records the restored
// start as an instant, on a live meeting and on one archived since.
func TestTheNoonRepairRecordsTheRestoredStartAsAnInstant(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	live := f.book(bookedAtMorning, f.contact("Live Contact"))
	f.save(live, berlinNoon)
	archived := f.book(bookedAtMorning, f.contact("Archived Contact"))
	f.save(archived, berlinNoon)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatalf("archiving the meeting: %v", err)
	}
	f.cutoffNow()

	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())

	for name, meeting := range map[string]ids.ActivityID{"live": live, "archived": archived} {
		if got := f.historyStart(meeting); got != "2026-07-15T07:00:00.123Z" {
			t.Errorf("the %s meeting's held start = %s, want the restored start", name, got)
		}
		if got := f.precisions(meeting); !slices.Equal(got, []string{"instant", "instant"}) {
			t.Errorf("the %s meeting's history precisions = %v, want the booking and the restored hold as instants", name, got)
		}
	}
}

// Upgraded after the repair ran: its restored rows recorded no precision. The
// precision task proves them instants from the repair's audit rows and leaves
// every other old row unknown. Run twice it changes nothing, and a later edit
// records its own precision.
func TestThePrecisionTaskProvesOnlyStartsTheNoonRepairRestored(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	shared := f.contact("Shared Contact")
	damaged := f.book(bookedAtMorning, shared)
	f.save(damaged, berlinNoon)
	reallyAtNoon := f.book(berlinNoon, shared)
	f.save(reallyAtNoon, berlinNoon)
	fromADay := f.bookDay(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), shared)
	f.cutoffNow()
	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())
	forgetMeetingPrecision(t, integration.OwnerConn(t), damaged, reallyAtNoon, fromADay)
	restoredVersion := f.version(damaged)

	task := meetingHistoryPrecisionDataTask()
	runMachineAddressTask(ctx, t, e, task)

	if got := f.precisions(damaged); !slices.Equal(got, []string{"instant", "instant"}) {
		t.Errorf("the repaired meeting's precisions = %v, want both rows proved instants", got)
	}
	if got := f.historyStart(damaged); got != "2026-07-15T07:00:00.123Z" {
		t.Errorf("the repaired meeting's held start moved to %s", got)
	}
	if got := f.precisions(reallyAtNoon); !slices.Equal(got, []string{"unknown", "unknown"}) {
		t.Errorf("a meeting really held at noon reads %v, want unknown: nothing proves its noon was a time", got)
	}
	if got := f.precisions(fromADay); !slices.Equal(got, []string{"unknown"}) {
		t.Errorf("a day meeting reads %v, want unknown", got)
	}

	rerun := task
	rerun.Version++
	runMachineAddressTask(ctx, t, e, rerun)
	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())
	if got := f.version(damaged); got != restoredVersion {
		t.Errorf("a second run moved the repaired meeting to version %d", got)
	}
	if got := f.precisions(reallyAtNoon); !slices.Equal(got, []string{"unknown", "unknown"}) {
		t.Errorf("a second run guessed %v for the noon meeting", got)
	}

	noShow := string(crmcontracts.ActivityMeetingStatusNoShow)
	f.move(e.Admin(), damaged, bookedAtMorning, &noShow)
	if got := f.precisions(damaged); !slices.Equal(got, []string{"instant", "instant", "instant"}) {
		t.Errorf("after a later edit the precisions are %v, want the new transition to record its own", got)
	}
}
