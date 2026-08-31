// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Display names for a SET of this module's records, one query per shape.
//
// The attention feed names every subject on the page, and the page can carry
// a hundred people. Read one at a time that is a hundred gated round trips
// whose count grows with how busy the workspace is; read together it is one
// query whose cost grows with the same page.
//
// Each read carries the object grant and the row-scope clause its
// single-record get carries, so a name is exactly as visible as the record.
// A record the caller may not see is ABSENT from the answer rather than
// refused: the caller asked about a set, and one unreadable member is not a
// failure of the question.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PersonLabels answers each named person's display name, under the caller's
// own grants. The column is full_name — the field personFace puts on a card —
// and a person with none is absent rather than blank.
func (s *Store) PersonLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, entityPerson, "person", "full_name", want)
}

// OrganizationLabels answers each named company's display name.
func (s *Store) OrganizationLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, entityOrganization, "organization", "display_name", want)
}

// LeadLabels answers each named lead's display name. A lead captured without
// one is absent, which is what its face does with an empty label.
func (s *Store) LeadLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	return s.labelsOf(ctx, entityLead, "lead", "full_name", want)
}

// labelsOf is the one read the three shapes above are: the same grant, the
// same row scope, the same archived exclusion, differing only in which table
// and which column carries the name.
//
// The table and column are constants from the callers above and never reach
// this from a request, which is what makes the format string safe here; the
// ids travel as a parameter like every other value.
func (s *Store) labelsOf(ctx context.Context, object, table, column string, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope, err := auth.ScopeClauseFor(ctx, object, "r", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		found, err := storekit.LabelsByID(ctx, tx, fmt.Sprintf(`
			SELECT r.id, coalesce(r.%s, '')
			  FROM %s r
			 WHERE r.id = ANY($%d) AND r.archived_at IS NULL AND (%s)`,
			column, table, idsPos, scope), args...)
		labels = found
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("people: reading %s display names: %w", table, err)
	}
	return labels, nil
}
