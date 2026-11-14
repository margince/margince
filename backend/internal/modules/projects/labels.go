// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Display names for a SET of projects, in one query.
//
// The attention feed names every subject on the page; read one at a time that
// is a round trip per card. It carries the object grant and the row-scope
// clause GetProject carries, so a name is exactly as visible as the project,
// and a project the caller cannot see is absent rather than refused.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProjectLabels answers each named project's own name, under the caller's
// grants.
func (s *Store) ProjectLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	query, args, err := projectIdentityQuery(ctx, want, storekit.LiveOnly, false)
	if err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		labels, err = storekit.LabelsByID(ctx, tx, query, args...)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("projects: reading project names: %w", err)
	}
	return labels, nil
}

// ProjectReferences returns scoped project names and archive state without full detail reads.
func (s *Store) ProjectReferences(ctx context.Context, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	query, args, err := projectIdentityQuery(ctx, want, storekit.IncludeArchived, true)
	if err != nil {
		return nil, err
	}
	references := []crmcontracts.RecordReference{}
	if len(want) == 0 {
		return references, nil
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		references, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.RecordReference, error) {
			var reference crmcontracts.RecordReference
			var id ids.UUID
			err := row.Scan(&id, &reference.Name, &reference.Archived)
			reference.Id = openapi_types.UUID(id)
			return reference, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("projects: reading project references: %w", err)
	}
	return references, nil
}

// Legacy labels keep their two-column projection and existing collector.
func projectIdentityQuery(ctx context.Context, want []ids.UUID, archived storekit.ArchivedFilter, reference bool) (string, []any, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return "", nil, err
	}
	if len(want) == 0 {
		return "", nil, nil
	}
	name := "p.name"
	columns := "p.id, coalesce(" + name + ", '')"
	if reference {
		columns = "p.id, " + name + ", p.archived_at IS NOT NULL"
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope, err := auth.ScopeClauseFor(ctx, projectObject, "p", arg)
	if err != nil {
		return "", nil, err
	}
	if scope == "" {
		scope = "true"
	}
	live := ""
	if archived == storekit.LiveOnly {
		live = " AND p.archived_at IS NULL"
	}
	query := fmt.Sprintf(`SELECT %s FROM %s p WHERE p.id = ANY($%d) AND (%s)%s`, columns, projectObject, idsPos, scope, live)
	return query, args, nil
}
