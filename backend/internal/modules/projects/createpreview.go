// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func admitProjectCreation(ctx context.Context, tx pgx.Tx, in CreateProjectInput, fallback *ids.UserID) (CreateProjectInput, error) {
	owner, err := storekit.NewRecordOwnerWithDefault(ctx, tx, in.OwnerID, fallback)
	if err != nil {
		return CreateProjectInput{}, err
	}
	in.OwnerID = owner
	if err := auth.EnsureReferenceTarget(ctx, tx, "company_id", "company", in.CompanyID.UUID); err != nil {
		return CreateProjectInput{}, err
	}
	if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
		return CreateProjectInput{}, err
	}
	if err := refuseStartPhase(in.Phase); err != nil {
		return CreateProjectInput{}, err
	}
	if in.Key != nil {
		key, err := normalizeChosenKey(*in.Key)
		if err != nil {
			return CreateProjectInput{}, err
		}
		if err := refuseChosenKey(ctx, tx, key); err != nil {
			return CreateProjectInput{}, err
		}
		in.Key = &key
	}
	return in, nil
}

// startPhase is the phase the project is born in.
func (in CreateProjectInput) startPhase() string {
	if in.Phase == "" {
		return PhaseInitiative
	}
	return in.Phase
}

// refuseStartPhase admits every phase but closed: closing is a claim that the
// work ended, which needs a reason and a project that existed to end.
func refuseStartPhase(phase string) error {
	switch phase {
	case "", PhaseInitiative, PhasePursuing, PhaseDelivering:
		return nil
	default:
		return &ProjectStartPhaseError{}
	}
}

// ProjectStartPhaseError maps to 422: a project starts in initiative, pursuing
// or delivering.
type ProjectStartPhaseError struct{}

func (e *ProjectStartPhaseError) Error() string {
	return "a new project starts in initiative, pursuing or delivering; close it after it is created"
}

// FieldFault names the create body's phase field.
func (e *ProjectStartPhaseError) FieldFault() (field, code, message string) {
	return "phase", "invalid_phase", e.Error()
}

// PreviewProjectCreationForActor leaves key allocation to the writer rather than reserving it in review.
func (s *Store) PreviewProjectCreationForActor(ctx context.Context, in CreateProjectInput, creator principal.Principal) (CreateProjectInput, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return CreateProjectInput{}, err
	}
	creatorCtx := principal.WithActor(ctx, creator)
	if err := auth.Require(creatorCtx, projectObject, principal.ActionCreate); err != nil {
		return CreateProjectInput{}, err
	}
	if _, err := storekit.CapturedBy(creatorCtx); err != nil {
		return CreateProjectInput{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return CreateProjectInput{}, err
	}
	var out CreateProjectInput
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = admitProjectCreation(ctx, tx, in, storekit.OwnerOrActor(creatorCtx))
		return err
	})
	if err != nil {
		return CreateProjectInput{}, err
	}
	out.CustomFields, err = storekit.CreateFieldValues(active, in.CustomFields)
	return out, err
}
