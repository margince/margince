// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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

// foldedPair stores the duplicates task's proven pair: an import naming Rep2,
// held by Rep1's calendar, and Rep2's calendar copy.
func foldedPair(t *testing.T, e *integration.Env) (held, copyID ids.UUID) {
	t.Helper()
	held = importMeeting(t, e, "human:"+e.Rep1.String())
	e.WsExec(t, `UPDATE activity SET host_user_id = $1 WHERE id = $2`, e.Rep1, held)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, held, rep2Address)
	return held, seedCalendarCopy(t, e, e.Rep2, rep2Address)
}

func liveMeeting(t *testing.T, e *integration.Env, id ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, id) == 1
}

// A verdict decides who may read a meeting as much as a posture does. A copy
// whose seat, or whose other importer, holds another verdict than on the
// survivor is not folded: the stricter verdict would vanish with it.
func TestAMeetingCopyWithAnotherVerdictIsNotFolded(t *testing.T) {
	for name, oppose := range map[string]func(e *integration.Env, held, copyID ids.UUID){
		"the folding seat": func(e *integration.Env, held, copyID ids.UUID) {
			e.WsExec(t, `UPDATE capture_import SET verdict_status = 'held' WHERE activity_id = $1`, copyID)
			e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
				VALUES ($1, $2, 'held', 'cleared')`, held, e.Rep2)
		},
		"another importer": func(e *integration.Env, held, copyID ids.UUID) {
			e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, verdict_status) VALUES ($1, $2, 'held')`, copyID, e.Rep1)
			e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, verdict_status) VALUES ($1, $2, 'cleared')`, held, e.Rep1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			held, copyID := foldedPair(t, e)
			oppose(e, held, copyID)

			runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))

			if !liveMeeting(t, e, copyID) {
				t.Error("the copy was folded though an importer's verdict differs on the survivor")
			}
		})
	}
}

// A human's outcome written on the copy while the fold waits for its lock
// keeps the copy. Eligibility is judged only once both rows are locked.
func TestAHumanOutcomeCommittedWhileTheFoldWaitsBlocksIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	_, copyID := foldedPair(t, e)

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	human, err := e.Pool.Begin(wsCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := human.Rollback(wsCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := human.Exec(wsCtx, `SELECT 1 FROM activity WHERE id = $1 FOR UPDATE`, copyID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))
	}()
	waitForALockWait(t, e)
	if _, err := human.Exec(wsCtx, `INSERT INTO audit_log (action, entity_type, entity_id, actor_type, actor_id, after)
		VALUES ('update', 'activity', $1, 'human', $2, '{"meeting_status":"held"}')`, copyID, "human:"+e.Rep2.String()); err != nil {
		t.Fatal(err)
	}
	if err := human.Commit(wsCtx); err != nil {
		t.Fatal(err)
	}
	<-done

	if !liveMeeting(t, e, copyID) {
		t.Error("the fold retired a copy a human recorded an outcome on while it waited")
	}
}

// waitForALockWait returns once some session waits on a row lock. Each check
// pauses inside the database, so the test reads no clock of its own.
func waitForALockWait(t *testing.T, e *integration.Env) {
	t.Helper()
	for range 400 {
		if _, err := e.Pool.Exec(context.Background(), `SELECT pg_sleep(0.025)`); err != nil {
			t.Fatal(err)
		}
		var waiting bool
		if err := e.Pool.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatal("the fold never waited for the copy's row lock")
}

// A folded copy's archive is not undone from history: its identities and
// guests now belong to the meeting that stayed.
func TestAFoldedMeetingCopyIsNotRestoredFromHistory(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	_, copyID := foldedPair(t, e)
	runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))
	if liveMeeting(t, e, copyID) {
		t.Fatal("the proven copy was not folded")
	}

	err := undoEntry(t, e, entityTypeActivity, copyID, latestAuditRowID(t, e, entityTypeActivity, copyID, actionArchive))
	if reason := refusedFor(t, err); reason != ReasonNotRestorableByThisPath {
		t.Errorf("undoing the fold answered %q, want %q", reason, ReasonNotRestorableByThisPath)
	}
	if liveMeeting(t, e, copyID) {
		t.Error("the folded copy is live again")
	}
}

// After a fold both seats stay busy for the meeting's hour, and the survivor
// lists the guest only the copy named.
func TestAFoldedMeetingKeepsBothSeatsBusyAndEveryGuest(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	held, copyID := foldedPair(t, e)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'extra@acme.test', 'attendee')`, copyID)

	runCalendarDescriptionTask(ctx, t, e, meetingPairsDataTask(e.Pool))

	if liveMeeting(t, e, copyID) {
		t.Fatal("the proven copy was not folded")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = 'extra@acme.test'`, held); got != 1 {
		t.Errorf("the survivor lists the copy's extra guest %d times, want once", got)
	}
	for name, seat := range map[string]ids.UUID{"the host": e.Rep1, "the copy's seat": e.Rep2} {
		if slotIsFree(t, e, seat, meetingStart) {
			t.Errorf("%s shows the folded meeting's hour as free", name)
		}
	}
}

// slotIsFree answers whether seat's availability offers the hour at start.
func slotIsFree(t *testing.T, e *integration.Env, seat ids.UUID, start time.Time) bool {
	t.Helper()
	slots, _, err := e.Activities.Availability(e.Admin(), ids.From[ids.UserKind](seat), start.Add(-time.Hour), start.Add(3*time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("availability: %v", err)
	}
	for _, s := range slots {
		if s.Start.Equal(start) {
			return true
		}
	}
	return false
}
