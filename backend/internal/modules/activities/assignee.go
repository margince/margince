// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who may be given a task.
//
// Its own file because it is its own question: lifecycle.go answers what a
// patch may change about an activity, and this answers whether the seat named
// can hold work at all. Both doors — the create and the patch — ask it, which is
// the other reason it does not belong inside either.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ensureAssigneeCanHoldWork checks a client-supplied user reference before it
// lands: the FK checks existence, RLS the tenancy. Nil means the caller does
// not touch the assignee, which is not this function's to gate.
//
// It asks auth.EnsureTaskAssignee, the rule a record's owner meets: 422 on
// assignee_id, one code for every reason. An agent seat opens no Worklist, so
// work assigned to it would read as delegated while being abandoned.
func ensureAssigneeCanHoldWork(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) error {
	_, err := taskSeat(ctx, tx, assigneeID, false)
	return err
}

// ensureNewTaskAssignee is ensureAssigneeCanHoldWork for a new task, where an
// invited colleague is also a valid assignee. Their task waits in their
// Worklist until they sign in, as a new lead does for its invited owner.
func ensureNewTaskAssignee(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) error {
	_, err := taskSeat(ctx, tx, assigneeID, true)
	return err
}

// CheckNewTaskAssignee is ensureNewTaskAssignee for a writer outside this
// module: the task form's rule, so a record filed by a unit meets the same one.
func CheckNewTaskAssignee(ctx context.Context, tx pgx.Tx, seat ids.UserID) error {
	return ensureNewTaskAssignee(ctx, tx, &seat)
}

// taskSeat asks one of the two seat questions above and returns the seat's
// status.
func taskSeat(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID, newTask bool) (string, error) {
	if assigneeID == nil {
		return "", nil
	}
	return auth.EnsureTaskAssignee(ctx, tx, assigneeID.UUID, newTask)
}

// SeatWorkState is whether a seat may be given a new task, and whether it has
// signed in yet.
type SeatWorkState int

const (
	// SeatCannotHoldWork is a seat that is missing, deactivated, archived, a read seat or an agent.
	SeatCannotHoldWork SeatWorkState = iota
	// SeatActive is a colleague who has signed in.
	SeatActive
	// SeatInvited is a colleague who has not signed in yet; their work waits.
	SeatInvited
)

// NewTaskSeatState answers, for a writer about to create a task, the question
// creating a task by hand asks of its assignee: an invited colleague counts.
//
// Asking rather than writing and recovering. A refused write aborts the
// caller's transaction, and a writer that fell back would continue inside a
// transaction Postgres has given up on. It is the create door's own call, so it
// cannot answer "yes" to a seat the write then refuses.
func NewTaskSeatState(ctx context.Context, tx pgx.Tx, seat ids.UserID) (SeatWorkState, error) {
	status, err := taskSeat(ctx, tx, &seat, true)
	if err == nil {
		if status == "invited" {
			return SeatInvited, nil
		}
		return SeatActive, nil
	}
	if _, ok := errors.AsType[*auth.AssigneeNotAllowedError](err); ok {
		return SeatCannotHoldWork, nil
	}
	return SeatCannotHoldWork, err
}

// SeatCanHoldAutomaticWork is NewTaskSeatState as a yes or no, for the card's
// "assignee inactive" state.
func SeatCanHoldAutomaticWork(ctx context.Context, tx pgx.Tx, seat ids.UserID) (bool, error) {
	state, err := NewTaskSeatState(ctx, tx, seat)
	return state != SeatCannotHoldWork, err
}

// fieldAssignee is the wire name of the column this module points a caller at.
const fieldAssignee = "assignee_id"

// A task with no assignee named belongs to the human behind the call: the
// writer, or the one an agent or connector acts for. Automatic work stays
// unassigned, the product's own job for a person included: it names its
// assignee, and none named means the work has no holder.
func taskAssignee(ctx context.Context, in LogActivityInput) *ids.UserID {
	if in.AssigneeID != nil || in.Kind != string(crmcontracts.ActivityKindTask) {
		return in.AssigneeID
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil
	}
	var human ids.UUID
	switch actor.Type {
	case principal.PrincipalHuman:
		human = actor.UserID
	case principal.PrincipalAgent, principal.PrincipalConnector:
		if actor.ProductAgentForItsUser() {
			return nil
		}
		human = actor.OnBehalfOf
	default:
		return nil
	}
	if human.IsZero() {
		return nil
	}
	owner := ids.From[ids.UserKind](human)
	return &owner
}

func checkedTaskAssignee(ctx context.Context, tx pgx.Tx, in LogActivityInput) (*ids.UserID, error) {
	assignee := taskAssignee(ctx, in)
	check := ensureAssigneeCanHoldWork
	if in.invitedAssignee {
		check = ensureNewTaskAssignee
	}
	if err := check(ctx, tx, assignee); err != nil {
		return nil, err
	}
	return assignee, nil
}
