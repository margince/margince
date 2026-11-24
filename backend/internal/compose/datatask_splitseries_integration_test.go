// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// storedSplitPair captures one slot of series uid-a and then of uid-b on the
// admin's calendar. The sink does not fold split series, as capture stored
// them before it did. It answers the old and the new meeting.
func storedSplitPair(t *testing.T, e *integration.Env, organizers [2]string) (ids.UUID, ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('gcal',$1,'connected','admin@calendar.test')`,
		e.AdminUser)
	slot := time.Now().UTC().AddDate(0, 0, -3).Truncate(time.Hour)
	var stored [2]ids.UUID
	for i, series := range []string{"uid-a", "uid-b"} {
		stored[i] = integration.CalendarMeeting{
			Event: series + "_slot", At: slot,
			Identity: connector.CrossDoorIdentity{Series: series, Occurrence: slot},
			Parties: []connector.MessageParticipant{
				{Email: organizers[i], Role: connector.ParticipantRoleOrganizer},
				{Email: splitBuyer, Role: "attendee"},
			},
		}.Capture(t, e)
	}
	return stored[0], stored[1]
}

func TestTheSplitSeriesTaskFoldsOnlyAPairThatIsOneMeeting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		organizers [2]string
		worked     bool
		folded     bool
	}{
		{name: "one meeting in two series", organizers: [2]string{splitHost, splitHost}, folded: true},
		{name: "another organizer", organizers: [2]string{splitHost, splitPeer}},
		{name: "a member recorded the old meeting's outcome", organizers: [2]string{splitHost, splitHost}, worked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			old, current := storedSplitPair(t, e, tc.organizers)
			if tc.worked {
				e.RecordHeld(t, old)
			}

			runCalendarDescriptionTask(ctx, t, e, splitSeriesDataTask(e.Pool))
			runCalendarDescriptionTask(ctx, t, e, splitSeriesDataTask(e.Pool))

			oldLive := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, old)
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, current); got != 1 {
				t.Fatal("the new series' meeting was retired")
			}
			if tc.folded != (oldLive == 0) {
				t.Fatalf("the old series' meeting live = %d, want folded = %v", oldLive, tc.folded)
			}
			if !tc.folded {
				return
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
				AND actor_type = 'system' AND after ? 'archived_at'`, old); got != 1 {
				t.Errorf("the fold wrote %d archive audit rows, want 1", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity_identity WHERE activity_id = $1`, current); got != 2 {
				t.Errorf("the new meeting holds %d identities, want both series' keys", got)
			}
		})
	}
}
