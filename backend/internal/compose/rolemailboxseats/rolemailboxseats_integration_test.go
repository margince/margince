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
	}{{e.Rep1, own}, {e.Rep2, "sales@forecast.test"}, {e.Rep3, "alias-rep3@forecast.test"}} {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response, roster_seat)
			VALUES ($1, 'attendee', $2, $3, 'accepted', $3)`, meeting, answer.address, answer.seat)
	}
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
	if kept := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = 'sales@forecast.test' AND response = 'accepted' AND user_id IS NULL`, meeting); kept != 1 {
		t.Errorf("the role mailbox's answer rows = %d, want the answer kept with no seat", kept)
	}
	if logged := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`, meeting); logged != 1 {
		t.Errorf("audit rows = %d, want 1", logged)
	}
	if out := run(); out.Changed != 0 {
		t.Errorf("a second run changed %d meetings, want a replay to change none", out.Changed)
	}
}
