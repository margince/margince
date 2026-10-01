// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The two membership sections: the tags applied to the account, and the
// lists it belongs to. Tags are workspace-shared; lists carry an owner, so
// only the ones the caller can read are named.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// tagsSection reads the tags applied to the account.
//
// A SECOND reader of the same fact, kept deliberately and only until the
// company panel moves onto GET /records/{type}/{id}/tags — the one read that
// serves contact, company and deal alike and carries the assigner this one
// cannot. Removing it here first would break the shipped panel, which still
// draws from this block. It goes when the panel does.
func tagsSection(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]crmcontracts.Tag, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name, t.color, t.created_at, t.updated_at, t.archived_at
		FROM tag t
		JOIN taggable g ON g.tag_id = t.id AND g.entity_type = 'company' AND g.entity_id = $1
		WHERE t.archived_at IS NULL
		ORDER BY t.name, t.id
		LIMIT $2`, companyID, sectionLimit+1)
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

func (a *assembly) readTags() error {
	if err := auth.Require(a.ctx, "tag", principal.ActionRead); err != nil {
		return err
	}
	tags, err := tagsSection(a.ctx, a.tx, a.companyID)
	if err != nil {
		return err
	}
	a.out.Tags = &tags
	return nil
}

// ShowListMemberships makes the page name the Shortlists the account is on.
func (s *Service) ShowListMemberships() {
	s.lists = true
}

// readListMemberships names the Shortlists the account is on that the caller
// may find. Absent, not omitted, while lists are switched off.
func (a *assembly) readListMemberships() error {
	if !a.svc.lists {
		return nil
	}
	if err := auth.Require(a.ctx, "list", principal.ActionRead); err != nil {
		return err
	}
	lists, err := collections.ShortlistsHolding(a.ctx, a.tx, "company", a.companyID.UUID, sectionLimit+1)
	if err != nil {
		return err
	}
	a.out.ListMemberships = &lists
	return nil
}
