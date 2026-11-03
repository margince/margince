// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedGuestHostedMeeting stores a meeting as a guest-first sync left it: Rep2
// hosts it, Rep1 is its bound organizer, and both seats' calendars import it.
func seedGuestHostedMeeting(t *testing.T, e *integration.Env, capturedBy string) ids.UUID {
	t.Helper()
	meeting := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, host_user_id, source, captured_by)
		VALUES ($1, 'meeting', 'Quarterly review', $2, $3, 'manual', $4)`, meeting, meetingStart, e.Rep2, capturedBy)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'organizer'), ($1, $3, 'attendee')`,
		meeting, e.Rep1, e.Rep2)
	e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2), ($1, $3)`, meeting, e.Rep1, e.Rep2)
	return meeting
}

// The task hands a stored calendar meeting to its organizer once, with one
// audit row. A meeting a human logged, or one the app booked, keeps its host:
// there the host is evidence of who held it.
func TestTheOrganizerHostTaskRepairsAFoldedMeeting(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	synced := seedGuestHostedMeeting(t, e, "connector:gcal:"+e.Rep2.String())
	logged := seedGuestHostedMeeting(t, e, "human:"+e.Rep2.String())
	booked := seedGuestHostedMeeting(t, e, "connector:gcal:"+e.Rep2.String())
	e.WsExec(t, `INSERT INTO meeting_invitation (activity_id, host_user_id, provider, calendar_id, appointment, status, management_hash, management_ref)
		VALUES ($1, $2, 'gcal', 'primary', '{}', 'confirmed', $3, 'ref')`, booked, e.Rep2, ids.NewV7().String())

	runCalendarDescriptionTask(ctx, t, e, meetingOrganizerHostDataTask(e.Pool))
	runCalendarDescriptionTask(ctx, t, e, meetingOrganizerHostDataTask(e.Pool))

	for name, tc := range map[string]struct {
		meeting ids.UUID
		host    ids.UUID
	}{"the synced meeting": {synced, e.Rep1}, "the logged meeting": {logged, e.Rep2}, "the booked meeting": {booked, e.Rep2}} {
		if got := e.WsScalar(t, `SELECT host_user_id::text FROM activity WHERE id = $1`, tc.meeting); got != tc.host.String() {
			t.Errorf("%s is hosted by %s, want %s", name, got, tc.host)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'
		AND evidence->>'reason' = 'meeting_organizer_host'`, synced); got != 1 {
		t.Errorf("%d audit rows record the host move over two runs, want 1", got)
	}
}
