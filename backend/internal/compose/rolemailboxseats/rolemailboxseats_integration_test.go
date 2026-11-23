// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package rolemailboxseats_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/rolemailboxseats"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A stored calendar meeting holds three bound answers. Rep1 answered from their
// own address, Rep2 through a role mailbox, and Rep3 through a personal address
// the provider attested. After the task Rep2 is credited with nothing. Rep1 and
// Rep3 still are, and the role mailbox's answer stays on the meeting.
func TestTheTaskTakesTheSeatOffARoleMailboxAnswerAndLeavesTheRest(t *testing.T) {
	e := integration.Setup(t)
	meeting := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, meeting_status, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'meeting', 'outbound', 'held', 'Team call', now() - interval '3 days', 'role-call', 'manual', 'connector:google_calendar')`, meeting)
	own := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	for _, answer := range []struct {
		seat    ids.UUID
		address string
	}{{e.Rep1, own}, {e.Rep2, "info@forecast.test"}, {e.Rep3, "alias-rep3@forecast.test"}} {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response, roster_seat)
			VALUES ($1, 'attendee', $2, $3, 'accepted', $3)`, meeting, answer.address, answer.seat)
	}
	e.WsExec(t, `INSERT INTO activity_meeting_history (activity_id, status, actor, effective_at, credited_seats_at_change)
		VALUES ($1, 'held', 'human:x', now(), ARRAY[$2, $3, $4]::uuid[])`, meeting, e.Rep1, e.Rep2, e.Rep3)
	keepsTheirBinding := seedBindingsTheTaskMustKeep(t, e)
	credited := func(seat ids.UUID) int {
		return e.WsCount(t, `SELECT count(*) FROM activity a WHERE a.id = $1 AND `+storekit.MeetingCreditSQL("a", "$2"), meeting, seat)
	}
	if credited(e.Rep1) != 1 || credited(e.Rep2) != 1 || credited(e.Rep3) != 1 {
		t.Fatalf("before the task every bound answer credits its seat, got %d %d %d", credited(e.Rep1), credited(e.Rep2), credited(e.Rep3))
	}

	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	run := func() datatask.BatchOut {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = rolemailboxseats.DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := run(); out.Changed != 1 {
		t.Errorf("the task changed %d meetings, want 1", out.Changed)
	}
	if credited(e.Rep2) != 0 || credited(e.Rep1) != 1 || credited(e.Rep3) != 1 {
		t.Errorf("after the task the credit is %d %d %d, want 1 0 1 for Rep1, Rep2, Rep3", credited(e.Rep1), credited(e.Rep2), credited(e.Rep3))
	}
	if kept := e.WsCount(t, `SELECT count(*) FROM activity_meeting_history WHERE activity_id = $1 AND $2 = ANY(credited_seats_at_change)`, meeting, e.Rep2); kept != 0 {
		t.Errorf("snapshots still crediting the role mailbox's seat = %d, want none", kept)
	}
	if kept := e.WsCount(t, `SELECT count(*) FROM activity_meeting_history WHERE activity_id = $1 AND $2 = ANY(credited_seats_at_change) AND $3 = ANY(credited_seats_at_change)`, meeting, e.Rep1, e.Rep3); kept != 1 {
		t.Errorf("snapshots keeping the other two seats = %d, want 1", kept)
	}
	if bound := keepsTheirBinding(); bound != 3 {
		t.Errorf("bindings a person made or a seat's own address = %d still bound, want all 3", bound)
	}
	if kept := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = 'info@forecast.test' AND response = 'accepted' AND user_id IS NULL`, meeting); kept != 1 {
		t.Errorf("the role mailbox's answer rows = %d, want the answer kept with no seat", kept)
	}
	if logged := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`, meeting); logged != 1 {
		t.Errorf("audit rows = %d, want 1", logged)
	}
	if out := run(); out.Changed != 0 {
		t.Errorf("a second run changed %d meetings, want a replay to change none", out.Changed)
	}
}

// seedBindingsTheTaskMustKeep adds three bound rows the task must leave alone
// and returns how many are still bound. They are a seat whose own login is
// sales@, a row a member relinked, and an answer on a meeting a person logged.
func seedBindingsTheTaskMustKeep(t *testing.T, e *integration.Env) func() int {
	t.Helper()
	sales := ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'sales@forecast.test', 'Sales Person')`, sales)
	calendar, logged := ids.NewV7(), ids.NewV7()
	for id, stamp := range map[ids.UUID]string{calendar: "connector:google_calendar", logged: "human:x"} {
		e.WsExec(t, `INSERT INTO activity (id, kind, direction, meeting_status, subject, occurred_at, thread_key, source, captured_by)
			VALUES ($1, 'meeting', 'outbound', 'held', 'Other call', now() - interval '3 days', $1::text, 'manual', $2)`, id, stamp)
	}
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, 'attendee', 'sales@forecast.test', $2, 'accepted')`, calendar, sales)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response, relinked_at) VALUES ($1, 'attendee', 'info@forecast.test', $2, 'accepted', now())`, calendar, e.Rep2)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, 'attendee', 'info@forecast.test', $2, 'accepted')`, logged, e.Rep2)
	return func() int {
		return e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id IN ($1, $2) AND user_id IS NOT NULL`, calendar, logged)
	}
}
