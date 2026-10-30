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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ensureAssigneeCanHoldWork checks a client-supplied user reference before it
// lands: the FK checks existence, RLS the tenancy. Nil means the caller does
// not touch the assignee, which is not this function's to gate.
//
// AN AGENT SEAT IS REFUSED. It is an Agent Runner identity, not a colleague: it
// opens no Worklist, so work assigned to it leaves every human queue at once —
// the task lane reads per human — and reads as delegated while being in fact
// abandoned. The same posture identity.SetTeamMember takes on the same column,
// for the same reason: the seat is a credential, and neither a role nor a queue
// belongs on it.
//
// Both doors ask. The patch door has asked since it was written; the create door
// had not, so a task could be MINTED onto an agent seat and only fail to move
// afterwards.
func ensureAssigneeCanHoldWork(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) error {
	_, err := seatHoldingWork(ctx, tx, assigneeID,
		`SELECT is_agent, status FROM app_user WHERE id = $1 AND `+workforce.LiveMemberSQL(""))
	return err
}

// ensureNewTaskAssignee is ensureAssigneeCanHoldWork for a task being CREATED,
// where an invited colleague is also a valid assignee. An invited colleague is
// a real colleague who has not signed in yet, and a task created on their
// behalf (an import, most of all) waits in their Worklist until they do — the
// same rule a new lead's owner follows. Moving an EXISTING task keeps asking
// ensureAssigneeCanHoldWork, so nothing is re-routed to a seat nobody answers.
func ensureNewTaskAssignee(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) error {
	_, err := newTaskSeat(ctx, tx, assigneeID)
	return err
}

// CheckNewTaskAssignee is ensureNewTaskAssignee for a writer outside this
// module: the task form's rule, so a record filed by a unit meets the same one.
func CheckNewTaskAssignee(ctx context.Context, tx pgx.Tx, seat ids.UserID) error {
	return ensureNewTaskAssignee(ctx, tx, &seat)
}

// newTaskSeat is ensureNewTaskAssignee that also says whether the seat has
// signed in yet.
func newTaskSeat(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) (string, error) {
	return seatHoldingWork(ctx, tx, assigneeID,
		`SELECT is_agent, status FROM app_user WHERE id = $1 AND `+workforce.ActivatableMemberSQL(""))
}

// seatHoldingWork runs one of the two seat questions above and returns the
// seat's status.
func seatHoldingWork(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID, seatSQL string) (string, error) {
	if assigneeID == nil {
		return "", nil
	}
	var isAgent bool
	var status string
	err := tx.QueryRow(ctx, seatSQL, *assigneeID).Scan(&isAgent, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		// A seat that is not there, not active, or archived is answered the way
		// it always was: not found, indistinguishable from a guessed id.
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if isAgent {
		return "", &AgentAssigneeError{}
	}
	return status, nil
}

// SeatWorkState is whether a seat may be given a new task, and whether it has
// signed in yet.
type SeatWorkState int

const (
	// SeatCannotHoldWork is a seat that is missing, deactivated, archived or an agent.
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
	status, err := newTaskSeat(ctx, tx, &seat)
	if err == nil {
		if status == "invited" {
			return SeatInvited, nil
		}
		return SeatActive, nil
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		return SeatCannotHoldWork, nil
	}
	if _, ok := errors.AsType[*AgentAssigneeError](err); ok {
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

// AgentAssigneeError refuses work aimed at an agent seat.
//
// A field fault rather than a not-found: the seat EXISTS and the caller may well
// be able to see it, so answering "no such user" would send them looking for a
// typo. What is wrong is the choice, and the field pointer says which one.
type AgentAssigneeError struct{}

func (e *AgentAssigneeError) Error() string {
	return "an agent seat holds no queue, so it cannot be given a task — assign it to a colleague"
}

// FieldFault names the assignee: it is the field the caller has to change.
func (e *AgentAssigneeError) FieldFault() (field, code, message string) {
	return fieldAssignee, faultInvalid, e.Error()
}

// fieldAssignee is the wire name of the column this module points a caller at.
const fieldAssignee = "assignee_id"

// A task with no assignee named belongs to the human behind the call: the
// writer, or the one an agent or connector acts for. Automatic work stays unassigned.
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
