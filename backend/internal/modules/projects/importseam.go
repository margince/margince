// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The seams a CSV import lands projects through: a create on the importer's own
// transaction, and the create's refusals asked without writing.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CreateProjectTx is CreateProject on the caller's transaction, for a write
// that must commit or roll back with the caller's own. Custom fields ride on the
// columns the caller read before it opened the transaction (ActiveProjectColumns).
func (s *Store) CreateProjectTx(ctx context.Context, tx pgx.Tx, in CreateProjectInput, cols CustomColumns) (crmcontracts.Project, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return crmcontracts.Project{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Project{}, err
	}
	return createProjectTx(ctx, tx, in, by, cols.cols, s.createSeams())
}

// RefuseImportCreate answers every refusal CreateProjectTx would answer before
// its insert, and writes nothing. Those are the dates, the company, the owner,
// the start phase, the deal link and the custom-field values. The import's test
// run asks it, so a row is previewed as the commit will treat it.
func (s *Store) RefuseImportCreate(ctx context.Context, tx pgx.Tx, in CreateProjectInput, cols CustomColumns) error {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return err
	}
	if err := refuseTargetEndBeforeStart(in.StartedAt, in.TargetEndDate, targetEndDateColumn); err != nil {
		return err
	}
	admitted, err := s.admitForImport(ctx, tx, in)
	if err != nil {
		return err
	}
	if admitted.DealID != nil {
		if err := s.checkDeal(ctx, tx, *admitted.DealID, admitted.CompanyID); err != nil {
			return err
		}
	}
	_, err = storekit.CreateFieldValues(cols.cols, in.CustomFields)
	return err
}

// admitForImport is admitProjectCreation for a row whose company may be one the
// upload creates first. That company has the zero id and is not looked up. A deal
// that names a company already differs from it, as the commit will find.
func (s *Store) admitForImport(ctx context.Context, tx pgx.Tx, in CreateProjectInput) (CreateProjectInput, error) {
	if !in.CompanyID.IsZero() {
		return admitProjectCreation(ctx, tx, in, storekit.OwnerOrActor(ctx))
	}
	in, err := admitSourceAndOwner(ctx, tx, in, storekit.OwnerOrActor(ctx))
	if err != nil {
		return CreateProjectInput{}, err
	}
	return admitRest(ctx, tx, in)
}

// RefuseUpdate answers the refusals UpdateProject would answer from the stored
// project and the input alone. That is a target end before the start the project
// would then hold. It writes nothing, so the import's test run says what the
// commit will.
func RefuseUpdate(current crmcontracts.Project, in UpdateProjectInput) error {
	return refuseMovedTargetEnd(current, in)
}

// LockProjectLive takes the row lock a reviewed import holds while it checks the
// project's version. The project must be readable by the caller and not archived.
func (s *Store) LockProjectLive(ctx context.Context, tx pgx.Tx, id ids.ProjectID) error {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureReadable(ctx, tx, projectObject, id.UUID); err != nil {
		return err
	}
	var locked bool
	err := tx.QueryRow(ctx, `SELECT true FROM project WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock project %s: %w", id, err)
	}
	return nil
}
