// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// Settling a commitment: what the rep did with it, what they need, and what
// their lead said back.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// settableByRep are the states a person may put a commitment into.
//
// `missed` is absent on purpose: it is what the week's CLOSE writes over an
// open commitment, not something a rep declares about themselves. A rep who
// decides they will not do a thing drops it, which says something different
// and truer than being marked as having failed.
var settableByRep = []string{StateOpen, StateDone, StateDropped}

// SetState moves one of the caller's own commitments.
func (s *Store) SetState(ctx context.Context, commitmentID ids.UUID, state string) error {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return err
	}
	if !slices.Contains(settableByRep, state) {
		return &values.ParseError{
			Field: "state", Code: "unknown",
			Message: "a commitment is open, done or dropped",
		}
	}
	owner, err := planUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		planID, current, err := ownCommitmentTx(ctx, tx, commitmentID, owner)
		if err != nil {
			return err
		}
		// completed_at moves with the state, because the CHECK ties them: a
		// done commitment says when, and one reopened stops claiming to.
		var completed any
		if state == StateDone {
			completed = time.Now().UTC()
		}
		patch := storekit.NewPatch()
		patch.Set("state", current.State, state)
		patch.Set("completed_at", current.CompletedAt, completed)
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan_commitment",
			commitmentID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan_commitment", commitmentID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(planID), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{"commitments"},
		})
	})
}

// AskForHelp records what the caller needs from their lead on one commitment.
//
// Its own event, because this is the one change somebody else is meant to act
// on. Clearing the text (an empty ask) withdraws the request and emits the
// ordinary update — a withdrawal must not page a lead a second time.
func (s *Store) AskForHelp(ctx context.Context, commitmentID ids.UUID, ask string) error {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return err
	}
	text, err := bounded("help_requested", ask, proseBound)
	if err != nil {
		return err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		planID, current, err := ownCommitmentTx(ctx, tx, commitmentID, owner)
		if err != nil {
			return err
		}
		patch := storekit.NewPatch()
		patch.Set("help_requested", current.HelpRequested, text)
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan_commitment",
			commitmentID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan_commitment", commitmentID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		if text == "" {
			return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
				PlanId: openapi_types.UUID(planID), OwnerUserId: openapi_types.UUID(owner),
				ChangedFields: []string{"help_requested"},
			})
		}
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanHelpRequested{
			PlanId:       openapi_types.UUID(planID),
			CommitmentId: openapi_types.UUID(commitmentID),
			OwnerUserId:  openapi_types.UUID(owner),
		})
	})
}

// Respond records what the caller, as the rep's lead, said about one
// commitment.
//
// The second writer, and the only path that touches somebody else's row. It
// writes THREE columns together — the text, who wrote it and when — because the
// table's CHECK ties them: an answer with nobody behind it cannot be shown to
// the person who asked.
//
// It touches nothing else. A lead may answer a request; they may not settle a
// commitment, reword it or drop it, and there is no argument here by which
// they could.
func (s *Store) Respond(ctx context.Context, commitmentID ids.UUID, answer string) error {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return err
	}
	text, err := bounded("manager_response", answer, proseBound)
	if err != nil {
		return err
	}
	if text == "" {
		return &values.ParseError{
			Field: "manager_response", Code: "required",
			Message: "an answer needs saying in words",
		}
	}
	me, err := planUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		planID, owner, current, err := commitmentOwnerTx(ctx, tx, commitmentID)
		if err != nil {
			return err
		}
		// Their own commitment is not a thing to answer: the rep would be
		// writing a manager response to themselves, which the page has no way
		// to render and nobody asked for.
		if owner == me {
			return apperrors.ErrNotFound
		}
		if err := s.requireLeadOf(ctx, owner); err != nil {
			return err
		}
		// Three columns together: the CHECK ties them, and an answer with
		// nobody behind it cannot be shown to the person who asked.
		patch := storekit.NewPatch()
		patch.Set("manager_response", current.ManagerResponse, text)
		patch.Set("manager_user_id", current.ManagerUserID, me)
		patch.Set("responded_at", current.RespondedAt, time.Now().UTC())
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan_commitment",
			commitmentID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan_commitment", commitmentID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		// Addressed to the REP, not to the lead who wrote it. The event says a
		// rep's plan changed, and the rep is whose plan it is.
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(planID), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{"manager_response"},
		})
	})
}

// ownCommitmentTx resolves a commitment the CALLER owns, or reports not found.
//
// Ownership is the predicate, not a check after the read: a commitment on
// somebody else's plan is not a thing this caller may learn exists.
func ownCommitmentTx(
	ctx context.Context, tx pgx.Tx, commitmentID, owner ids.UUID,
) (ids.UUID, Commitment, error) {
	var planID ids.UUID
	var status string
	var current Commitment
	err := tx.QueryRow(ctx, `
		SELECT p.id, p.status, c.state, c.help_requested, c.manager_response,
		       c.manager_user_id, c.responded_at, c.completed_at
		  FROM weekly_plan_commitment c
		  JOIN weekly_plan p ON p.id = c.plan_id
		 WHERE c.id = $1 AND p.owner_id = $2`, commitmentID, owner).
		Scan(&planID, &status, &current.State, &current.HelpRequested,
			&current.ManagerResponse, &current.ManagerUserID,
			&current.RespondedAt, &current.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, Commitment{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ids.Nil, Commitment{}, fmt.Errorf("weeklyplan: reading the commitment: %w", err)
	}
	// A closed week is frozen into a review that has already been counted.
	if status != "open" {
		return ids.Nil, Commitment{}, &values.ParseError{
			Field: "week", Code: "week_closed",
			Message: "that week is closed and its outcome is recorded",
		}
	}
	return planID, current, nil
}

// commitmentOwnerTx answers whose commitment this is, for the lead's path.
func commitmentOwnerTx(
	ctx context.Context, tx pgx.Tx, commitmentID ids.UUID,
) (planID, owner ids.UUID, current Commitment, err error) {
	err = tx.QueryRow(ctx, `
		SELECT p.id, p.owner_id, c.manager_response, c.manager_user_id, c.responded_at
		  FROM weekly_plan_commitment c
		  JOIN weekly_plan p ON p.id = c.plan_id
		 WHERE c.id = $1`, commitmentID).
		Scan(&planID, &owner, &current.ManagerResponse,
			&current.ManagerUserID, &current.RespondedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, ids.Nil, Commitment{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ids.Nil, ids.Nil, Commitment{}, fmt.Errorf("weeklyplan: reading the commitment: %w", err)
	}
	return planID, owner, current, nil
}
