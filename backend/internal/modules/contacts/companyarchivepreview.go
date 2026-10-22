// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	companyArchiveRelationships = "(company_id = @record OR counterparty_company_id = @record) AND archived_at IS NULL"
	companyArchiveMemberships   = "entity_type = 'company' AND entity_id = @record"
)

func companyArchiveAllowed(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	if err := refuseIfAnchor(ctx, tx, id, "id", "it cannot be archived. Archive a different company, or edit this one on the company page"); err != nil {
		return err
	}
	return refuseIfSoleCompanyOnALiveProject(ctx, tx, id)
}

// PreviewCompanyArchive resolves authorized effects without applying the proposed change.
func (s *Store) PreviewCompanyArchive(ctx context.Context, id ids.CompanyID, version *int64) (storekit.ArchiveScope, error) {
	var out storekit.ArchiveScope
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionDelete); err != nil {
		return out, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, companyEntity, id.UUID); err != nil {
			return err
		}
		if err := previewFieldsVisible(ctx, companyEntity, "domains", "relationship_types"); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, companyEntity, id.UUID, version); err != nil {
			return err
		}
		if err := companyArchiveAllowed(ctx, tx, id); err != nil {
			return err
		}
		var err error
		out, err = storekit.PreviewArchiveScope(ctx, tx, id.UUID, companyArchiveRelationships, companyArchiveMemberships)
		return err
	})
	if err != nil {
		return storekit.ArchiveScope{}, err
	}
	return out, nil
}
