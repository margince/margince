// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// companySourceKey is the unique index on a company's (source_system, source_id).
const companySourceKey = "uq_company_source"

// OutsideID is the id another system gives a record, with the system that
// issued it.
type OutsideID struct {
	System string
	ID     string
}

// stageAdoptedSource stages an outside id onto a hand-made company. A company
// that already holds one is refused: stamping over it would take the record
// from the system that owns it.
func stageAdoptedSource(ctx context.Context, tx pgx.Tx, id ids.CompanyID, p *storekit.Patch, adopt *OutsideID) error {
	if adopt == nil {
		return nil
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return err
	}
	if err := provenance.AdmitSourceID(&adopt.System, &adopt.ID); err != nil {
		return err
	}
	var held bool
	if err := tx.QueryRow(ctx,
		`SELECT source_system IS NOT NULL OR source_id IS NOT NULL FROM company WHERE id = $1 FOR UPDATE`, id.UUID).Scan(&held); err != nil {
		return fmt.Errorf("read the company's outside id: %w", err)
	}
	if held {
		return fmt.Errorf("this company already has an id from another system: %w", apperrors.ErrConflict)
	}
	p.Set("source_system", nil, adopt.System)
	p.Set("source_id", nil, adopt.ID)
	return nil
}
