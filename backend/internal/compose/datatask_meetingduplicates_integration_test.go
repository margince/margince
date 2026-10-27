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
	e.WsExec(t, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', 'evt-matched', $1)`,
		matchedEventPayload(t, calendar, "client@acme.test", matchedGuests...))
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label) VALUES ('gcal', $1, 'connected', $2)`,
		seat, calendar)
	return copyID
}

// A calendar copy of a meeting the import already holds is retired when the
// import names the syncing seat by an address it proved. A sync today binds to
// it then. Any copy whose retirement would lose something stays live.
func TestTheMeetingDuplicatesTaskRetiresOnlyACleanProvenCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		unnamed bool
		alter   func(t *testing.T, e *integration.Env, copyID ids.UUID)
		retired bool
	}{
		{name: "proven and clean", retired: true},
		{name: "the import does not name the seat", unnamed: true},
		{name: "another seat imported the copy", alter: func(t *testing.T, e *integration.Env, copyID ids.UUID) {
			e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, copyID, e.Rep1)
		}},
		{name: "the copy is filed under a record the survivor is not", alter: func(t *testing.T, e *integration.Env, copyID ids.UUID) {
			contact := e.SeedContact(t, "Client at Acme", nil)
			e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, copyID, contact)
		}},
		{name: "a human recorded the copy's outcome", alter: func(t *testing.T, e *integration.Env, copyID ids.UUID) {
			e.WsExec(t, `INSERT INTO audit_log (action, entity_type, entity_id, actor_type, actor_id, after)
				VALUES ('update', 'activity', $1, 'human', $2, '{"meeting_status":"held"}')`, copyID, "human:"+e.Rep2.String())
		}},
		{name: "the seat's calendar is another account now", alter: func(t *testing.T, e *integration.Env, _ ids.UUID) {
			e.WsExec(t, `UPDATE capture_connection SET account_label = 'other@authz.test' WHERE provider = 'gcal' AND user_id = $1`, e.Rep2)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			held := importMeeting(t, e, "human:"+e.Rep1.String())
			e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'client@acme.test', 'organizer')`, held)
			if !tc.unnamed {
				e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, held, rep2Address)
			}
			copyID := seedCalendarCopy(t, e, e.Rep2, rep2Address)
			if tc.alter != nil {
				tc.alter(t, e, copyID)
			}
			if got := meetingDuplicatesEstimate(ctx, t, e); got != 1 {
				t.Fatalf("the estimate counts %d calendar copies, want 1", got)
			}

			runCalendarDescriptionTask(ctx, t, e, meetingDuplicatesDataTask(e.Pool))
			runCalendarDescriptionTask(ctx, t, e, meetingDuplicatesDataTask(e.Pool))

			live := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, copyID)
			importers := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, held)
			if !tc.retired {
				if live != 1 || importers != 0 {
					t.Errorf("the copy reads live=%d and the survivor gained %d importers, want it left alone", live, importers)
				}
				return
			}
			if live != 0 {
				t.Error("the proven calendar copy is still live")
			}
			if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2 AND posture_at_import = 'held'`, held, e.Rep2); importers != 1 || got != 1 {
				t.Errorf("the survivor has %d importers, %d of them the copy's seat with its hold; want that seat alone", importers, got)
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
