// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The two membership sections: the tags applied to the account, and the
// lists it belongs to. Tags are workspace-shared; lists carry an owner, so
// only the ones the caller can read are named.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// listMembershipsSection reads the live Shortlists the account is on, pruned
// to the ones the caller may find.
func listMembershipsSection(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]crmcontracts.List, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	listScope, err := scopeClause(ctx, "list", "l", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT l.id, l.name, l.entity_type, l.list_type, l.sharing, l.version, l.purpose,
		       l.owner_id, l.steward_id, l.team_id, l.created_at, l.updated_at
		FROM list l
		JOIN list_member m ON m.list_id = l.id AND m.entity_type = 'company' AND m.entity_id = $%d
		WHERE l.archived_at IS NULL AND (%s)
		ORDER BY l.name, l.id
		LIMIT %d`, companyPos, listScope, sectionLimit+1), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.List, error) {
		var l crmcontracts.List
		var id ids.UUID
		var ownerID, stewardID, teamID *ids.UUID
		var entityType, listType, sharing string
		if err := row.Scan(&id, &l.Name, &entityType, &listType, &sharing, &l.Version, &l.Purpose,
			&ownerID, &stewardID, &teamID, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return l, err
		}
		l.Id = openapi_types.UUID(id)
		l.EntityType = crmcontracts.ListEntityType(entityType)
		l.ListType = crmcontracts.ListListType(listType)
		l.Sharing = crmcontracts.ListSharing(sharing)
		l.Health = crmcontracts.ListHealthOk
		l.OwnerId, l.StewardId, l.TeamId = uuidPtr(ownerID), uuidPtr(stewardID), uuidPtr(teamID)
		return l, nil
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
	lists, err := listMembershipsSection(a.ctx, a.tx, a.companyID)
	if err != nil {
		return err
	}
	a.out.ListMemberships = &lists
	return nil
}
