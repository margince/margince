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

const (
	projectArchiveRelationships = "project_id = @record AND kind = 'project_stakeholder' AND archived_at IS NULL"
	projectArchiveMemberships   = "entity_type = 'project' AND entity_id = @record"
)

// PreviewProjectArchive checks the version before reporting edges the archive would retire.
func (s *Store) PreviewProjectArchive(ctx context.Context, id ids.ProjectID, version *int64) (storekit.ArchiveScope, error) {
	var out storekit.ArchiveScope
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, projectObject, id.UUID); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, projectObject, id.UUID, version); err != nil {
			return err
		}
		var err error
		out, err = storekit.PreviewArchiveScope(ctx, tx, id.UUID, projectArchiveRelationships, projectArchiveMemberships)
		return err
	})
	if err != nil {
		return storekit.ArchiveScope{}, err
	}
	return out, nil
}
