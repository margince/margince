// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Names for a set of teams or roles, one query per kind, so a page that names
// fifty of them asks twice rather than fifty times. Each read carries the gate
// the kind's own list carries, and a row it may not answer is absent.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamLabels names the live teams in a set. Teams are roster, which ListTeams
// shows to every member.
func (s *Service) TeamLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	return s.namesOf(ctx, "team", want)
}

// RoleLabels names the live roles in a set, under the role_admin read
// ListRoles carries.
func (s *Service) RoleLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, objectRoleAdmin, principal.ActionRead); err != nil {
		return nil, err
	}
	return s.namesOf(ctx, roleEntity, want)
}

// namesOf is safe to format because table is a constant from the two callers
// above and never a request value.
func (s *Service) namesOf(ctx context.Context, table string, want []ids.UUID) (map[ids.UUID]string, error) {
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		found, err := storekit.LabelsByID(ctx, tx, fmt.Sprintf(`
			SELECT r.id, coalesce(r.name, '')
			  FROM %s r
			 WHERE r.id = ANY($%d) AND r.archived_at IS NULL`,
			table, idsPos), args...)
		labels = found
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("identity: reading %s names: %w", table, err)
	}
	return labels, nil
}
