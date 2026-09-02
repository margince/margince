// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The two membership sections: the tags applied to the account, and the
// lists it belongs to. Tags are workspace-shared; lists carry an owner, so
// only the ones the caller can read are named.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tagsSection reads the tags applied to the account.
func tagsSection(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) ([]crmcontracts.Tag, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name, t.color, t.created_at, t.updated_at, t.archived_at
		FROM tag t
		JOIN taggable g ON g.tag_id = t.id AND g.entity_type = 'organization' AND g.entity_id = $1
		WHERE t.archived_at IS NULL
		ORDER BY t.name, t.id
		LIMIT $2`, orgID, sectionLimit+1)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.Tag, error) {
		var t crmcontracts.Tag
		var id ids.UUID
		if err := row.Scan(&id, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt, &t.ArchivedAt); err != nil {
			return t, err
		}
		t.Id = openapi_types.UUID(id)
		return t, nil
	})
}
