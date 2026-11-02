// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A calendar row that holds an identity of its own, beside the meeting its
// seat's calendar binds the event to today, is folded once. A second run finds
// nothing left to do.
func TestTheMeetingPairsTaskFoldsAStoredPairOnce(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	held := importMeeting(t, e, "human:"+e.Rep1.String())
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, held, rep2Address)
	copyID := seedCalendarCopy(t, e, e.Rep2, rep2Address)
	staleKey := activities.MeetingIdentityKey(importedSeries, meetingStart.Add(-time.Hour).Format(time.RFC3339))
	e.WsExec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'connector:gcal')`, staleKey, copyID)

	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))
	}

	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, copyID); got != 0 {
		t.Fatal("the calendar row is still live beside the meeting its seat binds to")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_identity WHERE activity_id = $1`, held); got != 2 {
		t.Errorf("the survivor holds %d identities, want its own and the retired row's", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`, held, e.Rep2); got != 1 {
		t.Errorf("the copy's seat imports the survivor %d times, want once", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		AND after ? 'archived_at'`, copyID); got != 1 {
		t.Errorf("the fold wrote %d archive audit rows across two runs, want 1", got)
	}
}

// A stored original whose own row holds the event's key binds to itself, so
// the task leaves it for the seat's next sync.
func TestTheMeetingPairsTaskSkipsAnOldStartOriginal(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	copyID := seedCalendarCopy(t, e, e.Rep2, rep2Address)
	key := activities.MeetingIdentityKey(importedSeries, meetingStart.Format(time.RFC3339))
	e.WsExec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'connector:gcal')`, key, copyID)
	other := importMeeting(t, e, "human:"+e.Rep1.String())

	runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))

	for _, id := range []ids.UUID{copyID, other} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, id); got != 1 {
			t.Errorf("meeting %v was retired, want both rows left as they are", id)
		}
	}
}
