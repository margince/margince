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

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	if assigneeID == nil {
		return nil
	}
	var isAgent bool
	err := tx.QueryRow(ctx,
		`SELECT is_agent FROM app_user WHERE id = $1 AND status = 'active' AND archived_at IS NULL`,
		*assigneeID).Scan(&isAgent)
	if errors.Is(err, pgx.ErrNoRows) {
		// A seat that is not there, not active, or archived is answered the way
		// it always was: not found, indistinguishable from a guessed id.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if isAgent {
		return &AgentAssigneeError{}
	}
	return nil
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
