// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedCalendarCopy leaves what a Google Calendar sync once filed beside a
// meeting another door held. It is a row under the event's own key, with no
// meeting identity, and the event's original is stored.
func seedCalendarCopy(t *testing.T, e *integration.Env, seat ids.UUID, calendar string) ids.UUID {
	t.Helper()
	copyID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id, source, captured_by)
		VALUES ($1, 'meeting', 'Consulting Monthly', $2, 'gcal', 'evt-matched', 'gcal:evt-matched', $3)`,
		copyID, meetingStart, "connector:gcal:"+seat.String())
	e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, posture_at_import) VALUES ($1, $2, 'held')`, copyID, seat)
	// A colleague also recorded on the copy, whom the binding rule did not prove.
	e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, copyID, e.Rep1)
	e.WsExec(t, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', 'evt-matched', $1)`,
		matchedEventPayload(t, calendar, "client@acme.test", matchedGuests...))
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label) VALUES ('gcal', $1, 'connected', $2)`,
		seat, calendar)
	return copyID
}

// A calendar copy of a meeting the import already holds is retired when the
// import names the syncing seat by an address it proved. A sync today binds to
// it then. When the import does not name the seat, nothing proves the two rows
// are one meeting, and both stay.
func TestTheMeetingDuplicatesTaskRetiresOnlyAProvenCalendarCopy(t *testing.T) {
	for name, named := range map[string]bool{"import names the seat": true, "import does not": false} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			held := importMeeting(t, e, "human:"+e.Rep1.String())
			e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'client@acme.test', 'organizer')`, held)
			if named {
				e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, held, rep2Address)
			}
			copyID := seedCalendarCopy(t, e, e.Rep2, rep2Address)
			if got := meetingDuplicatesEstimate(ctx, t, e); got != 1 {
				t.Fatalf("the estimate counts %d calendar copies, want 1", got)
			}

			runCalendarDescriptionTask(ctx, t, e, meetingDuplicatesDataTask(e.Pool))
			runCalendarDescriptionTask(ctx, t, e, meetingDuplicatesDataTask(e.Pool))

			live := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, copyID)
			carried := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND posture_at_import = 'held'`, held, e.Rep2)
			if !named {
				if live != 1 || carried != 0 {
					t.Errorf("an unproven copy reads live=%d carried=%d, want it left alone", live, carried)
				}
				return
			}
			if live != 0 {
				t.Error("the proven calendar copy is still live")
			}
			if carried != 1 {
				t.Errorf("the surviving meeting records the copy's seat %d times with its posture, want once", carried)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, held); got != 1 {
				t.Errorf("the surviving meeting gained %d importers, want only the proven seat", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, held); got != 1 {
				t.Error("the meeting holding the identity was retired")
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
				AND actor_type = 'system' AND after ? 'archived_at'`, copyID); got != 1 {
				t.Errorf("the retirement wrote %d audit rows, want 1", got)
			}
			if got := meetingDuplicatesEstimate(ctx, t, e); got != 0 {
				t.Errorf("after the repair the estimate still counts %d copies", got)
			}
		})
	}
}

func meetingDuplicatesEstimate(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := meetingDuplicatesDataTask(e.Pool).Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatal(err)
	}
	return count
}
