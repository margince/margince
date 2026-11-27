// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatWithGrant is a full seat of user at own-row scope, holding grant on tasks.
func (e *promiseEnv) seatWithGrant(user ids.UUID, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{"activity": grant, "contact": {Read: true}},
		},
	})
}

// seedUnassignedTask files an open task with no assignee under a contact that
// contactOwner owns.
func (e *promiseEnv) seedUnassignedTask(t *testing.T, contactOwner ids.UUID) ids.UUID {
	t.Helper()
	task := e.seedLinkedNote(t, contactOwner)
	e.exec(t, `UPDATE activity SET kind = 'task', is_done = false WHERE id = $1`, task)
	return task
}

func (e *promiseEnv) takeStore(hook TaskTakenHook) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).WithTaskTaken(hook)
}

func (e *promiseEnv) assigneeOf(t *testing.T, task ids.UUID) *ids.UUID {
	t.Helper()
	var assignee *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT assignee_id FROM activity WHERE id = $1`, task).Scan(&assignee); err != nil {
		t.Fatal(err)
	}
	return assignee
}

func (e *promiseEnv) versionOf(t *testing.T, task ids.UUID) int64 {
	t.Helper()
	var version int64
	if err := e.owner.QueryRow(context.Background(), `SELECT version FROM activity WHERE id = $1`, task).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func (e *promiseEnv) rowsOf(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A reader who may see a colleague's task and not change it can take it while
// nobody holds it. The take writes one audit row and one event.
func TestAReaderTakesAnUnassignedTaskTheyMayNotChange(t *testing.T) {
	e := setupPromises(t)
	task := e.seedUnassignedTask(t, e.other)
	var told []TaskTaken
	store := e.takeStore(func(_ context.Context, _ pgx.Tx, taken TaskTaken) error {
		told = append(told, taken)
		return nil
	})
	reader := e.seatWithGrant(e.rep, principal.ObjectGrant{Read: true, Update: true})
	id := ids.From[ids.ActivityKind](task)

	if _, err := store.UpdateActivity(reader, id, UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](e.rep))}); err == nil {
		t.Fatal("a plain patch by the reader succeeded, so the take proves nothing about write access")
	}
	audits := e.rowsOf(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, task)
	events := e.rowsOf(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'activity.updated'`)

	taken, err := store.TakeTask(reader, id, e.versionOf(t, task))
	if err != nil {
		t.Fatalf("taking the task: %v", err)
	}
	if got := e.assigneeOf(t, task); got == nil || *got != e.rep {
		t.Fatalf("assignee = %v, want the reader", got)
	}
	if taken.AssigneeId == nil || ids.UUID(*taken.AssigneeId) != e.rep {
		t.Errorf("the answer carries assignee %v, want the reader", taken.AssigneeId)
	}
	if got := e.rowsOf(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, task) - audits; got != 1 {
		t.Errorf("the take wrote %d audit rows, want 1", got)
	}
	if got := e.rowsOf(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'activity.updated'`) - events; got != 1 {
		t.Errorf("the take wrote %d activity.updated events, want 1", got)
	}
	if len(told) != 1 || told[0].Taker != ids.From[ids.UserKind](e.rep) {
		t.Errorf("the take hook ran %d times, want once for the reader", len(told))
	}

	if _, err := store.UpdateActivity(reader, id, UpdateActivityInput{IsDone: new(true)}); err != nil {
		t.Errorf("the taker cannot complete the task they took: %v", err)
	}
}

// A hook that fails undoes the take. The notice and the assignment commit
// together or not at all.
func TestATakeIsUndoneWhenItsHookFails(t *testing.T) {
	e := setupPromises(t)
	task := e.seedUnassignedTask(t, e.other)
	store := e.takeStore(func(context.Context, pgx.Tx, TaskTaken) error { return errors.New("notice not written") })
	reader := e.seatWithGrant(e.rep, principal.ObjectGrant{Read: true, Update: true})

	if _, err := store.TakeTask(reader, ids.From[ids.ActivityKind](task), e.versionOf(t, task)); err == nil {
		t.Fatal("the take succeeded although its hook failed")
	}
	if got := e.assigneeOf(t, task); got != nil {
		t.Errorf("assignee = %v after a failed hook, want none", got)
	}
}

// Two colleagues take one task at the version both saw: one wins and the other
// is refused, and the winner stays the assignee.
func TestTwoTakersOfOneTaskLeaveExactlyOneWinner(t *testing.T) {
	e := setupPromises(t)
	task := e.seedUnassignedTask(t, e.other)
	store := e.takeStore(nil)
	version := e.versionOf(t, task)
	id := ids.From[ids.ActivityKind](task)
	second := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Second')`, second, "second-"+second.String()+"@promises.test")
	takers := []ids.UUID{e.rep, second}

	errs := make([]error, len(takers))
	var wg sync.WaitGroup
	for i, user := range takers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = store.TakeTask(e.seatWithGrant(user, principal.ObjectGrant{Read: true, Update: true}), id, version)
		}()
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil && winner < 0:
			winner = i
		case err == nil:
			t.Fatal("both takers succeeded")
		case !errors.Is(err, apperrors.ErrVersionSkew) && !errors.Is(err, apperrors.ErrConflict):
			t.Errorf("the loser was refused with %v, want a conflict", err)
		}
	}
	if winner < 0 {
		t.Fatalf("neither taker succeeded: %v", errs)
	}
	if got := e.assigneeOf(t, task); got == nil || *got != takers[winner] {
		t.Errorf("assignee = %v, want the winning taker %s", got, takers[winner])
	}
}

func TestATakeRefusesWhatIsNotAnOpenUnheldTask(t *testing.T) {
	e := setupPromises(t)
	reader := e.seatWithGrant(e.rep, principal.ObjectGrant{Read: true, Update: true})
	store := e.takeStore(nil)
	for name, tc := range map[string]struct {
		prepare string
		want    error
	}{
		"a task a colleague holds": {`UPDATE activity SET assignee_id = $2 WHERE id = $1`, apperrors.ErrConflict},
		"a task already done":      {`UPDATE activity SET is_done = true, done_at = now() WHERE id = $1 AND $2::uuid IS NOT NULL`, apperrors.ErrConflict},
		"a website inquiry task":   {`UPDATE activity SET source_system = 'website_inquiry', source_id = 'submission-1' WHERE id = $1 AND $2::uuid IS NOT NULL`, apperrors.ErrConflict},
		"a note":                   {`UPDATE activity SET kind = 'note' WHERE id = $1 AND $2::uuid IS NOT NULL`, apperrors.ErrConflict},
	} {
		task := e.seedUnassignedTask(t, e.other)
		e.exec(t, tc.prepare, task, e.other)
		before := e.rowsOf(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, task)
		held := e.assigneeOf(t, task)

		_, err := store.TakeTask(reader, ids.From[ids.ActivityKind](task), e.versionOf(t, task))
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if after := e.rowsOf(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, task); after != before {
			t.Errorf("%s: the refused take wrote %d audit rows", name, after-before)
		}
		if got := e.assigneeOf(t, task); (got == nil) != (held == nil) || (got != nil && *got != *held) {
			t.Errorf("%s: the refused take changed the assignee to %v", name, got)
		}
	}
}

func TestATakeIsRefusedWithoutTheUpdateGrantOrSight(t *testing.T) {
	e := setupPromises(t)
	store := e.takeStore(nil)

	visible := e.seedUnassignedTask(t, e.other)
	viewOnly := e.seatWithGrant(e.rep, principal.ObjectGrant{Read: true})
	if _, err := store.TakeTask(viewOnly, ids.From[ids.ActivityKind](visible), e.versionOf(t, visible)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat without the update grant: err = %v, want permission denied", err)
	}

	hidden := e.seedUnassignedTask(t, e.other)
	e.exec(t, `UPDATE contact SET visibility = 'owner' WHERE id IN (SELECT contact_id FROM activity_link WHERE activity_id = $1)`, hidden)
	reader := e.seatWithGrant(e.rep, principal.ObjectGrant{Read: true, Update: true})
	if _, err := store.TakeTask(reader, ids.From[ids.ActivityKind](hidden), e.versionOf(t, hidden)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a task the reader cannot see: err = %v, want not found", err)
	}
	for _, task := range []ids.UUID{visible, hidden} {
		if got := e.assigneeOf(t, task); got != nil {
			t.Errorf("a refused take left assignee %v on %s", got, task)
		}
	}
}
