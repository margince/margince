// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Taking an unassigned task: the one change to a task that a reader with no
// write authority over it may make. The task has no holder to protect, so any
// seat that can see it and may work tasks may become its holder.

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// TaskTaken is a task that has just been taken, with the seat that took it.
type TaskTaken struct {
	Task  crmcontracts.Activity
	Taker ids.UserID
}

// TaskTakenHook runs inside the transaction that wrote the take, so what it
// writes commits or rolls back with the assignment. Compose supplies it,
// because telling the record's owner reaches the notices module.
type TaskTakenHook func(ctx context.Context, tx pgx.Tx, taken TaskTaken) error

// WithTaskTaken wires what happens in the take's transaction after the
// assignee is written.
func (s *Store) WithTaskTaken(hook TaskTakenHook) *Store {
	s.taskTaken = hook
	return s
}

// WithTaskTaken hands the hook to the handlers' store.
func (h Handlers) WithTaskTaken(hook TaskTakenHook) Handlers {
	h.store.WithTaskTaken(hook)
	return h
}

// TakeTask makes the caller the assignee of an open task nobody holds, pinned
// to the version the caller saw. It rides the patch writer. The seat check, the
// audit row and the activity.updated event are the ones a reassignment writes.
// Only the write-authority arm differs (admitWriter).
//
// Two takers of one version are serialized by the row lock. The second finds
// the task held and is refused with a conflict.
func (s *Store) TakeTask(ctx context.Context, id ids.ActivityID, version int64) (crmcontracts.Activity, error) {
	if err := MayTakeTasks(ctx); err != nil {
		return crmcontracts.Activity{}, err
	}
	actor, _ := principal.Actor(ctx)
	taker := new(ids.From[ids.UserKind](actor.UserID))
	at := s.now()
	in := UpdateActivityInput{AssigneeID: taker, IfVersion: &version, takesUnassigned: true, recordedAt: &at, recordZone: s.recordZone}
	var out crmcontracts.Activity
	err := s.txRetryingLockCycles(ctx, pgx.ReadCommitted, func(tx pgx.Tx) error {
		if err := admitTake(ctx, tx, id); err != nil {
			return err
		}
		var err error
		out, err = updateActivityInTx(ctx, tx, id, in)
		if err == nil && s.taskTaken != nil {
			err = s.taskTaken(ctx, tx, TaskTaken{Task: out, Taker: *taker})
		}
		return err
	})
	if err != nil {
		return out, err
	}
	return out, s.attachCustomValues(ctx, &out)
}

// admitWriter is the write-authority arm of a patch. A take asks only that the
// caller can read the live row, because it changes nothing a colleague holds.
func (in UpdateActivityInput) admitWriter(ctx context.Context, tx pgx.Tx, id ids.ActivityID, held bool) error {
	if !in.takesUnassigned {
		return auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held)
	}
	return auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID)
}

// admitTake locks the task and admits the take only for a reader who can see it.
// The task must still be open and held by nobody. A task the reader cannot see is
// not found before anything else is said about it.
func admitTake(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
		return err
	}
	current, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return err
	}
	return refuseUntakeable(current)
}

// TakeableTask reports whether a stored activity is an open task nobody holds
// and no writer routes. A website-inquiry task keeps its own flow, which also
// hands on its lead. The Worklist offers "Take it" by this same rule.
func TakeableTask(task crmcontracts.Activity) bool {
	if task.Kind != crmcontracts.ActivityKindTask || task.AssigneeId != nil || (task.IsDone != nil && *task.IsDone) {
		return false
	}
	if task.SourceSystem == nil {
		return true
	}
	if provenance.RoutedTaskSource(*task.SourceSystem) {
		return false
	}
	return task.SourceId == nil || !provenance.IsWebsiteInquiryTask(*task.SourceSystem, *task.SourceId)
}

// MayTakeTasks is the seat rule of a take, shared by the endpoint and the
// Worklist's offer. It asks for a human on a full seat whose role may change
// tasks. It skips the row-write check, because a task nobody holds has no
// owner to protect.
func MayTakeTasks(ctx context.Context) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

func refuseUntakeable(current crmcontracts.Activity) error {
	if !TakeableTask(current) {
		return apperrors.ErrConflict
	}
	return nil
}

// TakeTask serves POST /activities/{id}/take. It needs If-Match.
func (h Handlers) TakeTask(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.TakeTaskParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if version == nil {
		writeStoreErr(w, r, apperrors.ErrInvalidArgument)
		return
	}
	out, err := h.store.TakeTask(r.Context(), pathID[ids.ActivityKind](id), *version)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
