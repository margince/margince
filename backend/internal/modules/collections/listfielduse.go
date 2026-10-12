// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Which fields a Live List's filter names, read off the stored tree itself:
// what a list reports when one of them is retired, and what retiring a field
// leaves behind.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retiredColumns is the custom columns of one record type that a filter may
// still name but a builder no longer offers: the retired ones.
func (s *Store) retiredColumns(ctx context.Context, entityType string) (map[string]bool, error) {
	if s.catalog == nil {
		return map[string]bool{}, nil
	}
	filterable, err := s.catalog.FilterableColumns(ctx, entityType)
	if err != nil {
		return nil, fmt.Errorf("read the custom-field columns for %s: %w", entityType, err)
	}
	offerable, err := s.offerableCustomColumns(ctx, entityType)
	if err != nil {
		return nil, err
	}
	retired := map[string]bool{}
	for _, column := range filterable {
		if !offerable[column.Name] {
			retired[column.Name] = true
		}
	}
	return retired, nil
}

// retiredFieldsOf is the retired custom fields a Live List's filter names.
// A Shortlist names none.
func (s *Store) retiredFieldsOf(ctx context.Context, l listRow) ([]string, error) {
	if l.ListType != listTypeDynamic {
		return nil, nil
	}
	pred, err := predicateFromDefinition(l.Definition)
	if err != nil {
		return nil, err
	}
	retired, err := s.retiredColumns(ctx, l.EntityType)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, field := range storekit.FieldsNamed(pred) {
		if retired[field] {
			out = append(out, field)
		}
	}
	return out, nil
}

// LiveListsUsingField answers the live, unarchived Live Lists of one record
// type whose filter names field. A list this caller may find is named; the
// others are only counted, so a private list's name stays with its owner and
// steward. The caller gates who may ask: it is the question retiring a field
// raises, and the field's grant is not this module's.
func (s *Store) LiveListsUsingField(ctx context.Context, entityType, field string) (crmcontracts.CustomFieldLiveLists, error) {
	out := crmcontracts.CustomFieldLiveLists{Lists: []crmcontracts.CustomFieldLiveList{}}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		using, err := liveListsNaming(ctx, tx, entityType, field)
		if err != nil || len(using) == 0 {
			return err
		}
		findable, err := findableAmong(ctx, tx, using)
		if err != nil {
			return err
		}
		for _, l := range using {
			if !findable[l.ID] {
				out.UnseenCount++
				continue
			}
			out.Lists = append(out.Lists, crmcontracts.CustomFieldLiveList{
				Id: openapi_types.UUID(l.ID.UUID), Name: l.Name, Sharing: crmcontracts.CustomFieldLiveListSharing(l.Sharing),
			})
		}
		return nil
	})
	return out, err
}

// liveListsNaming reads every unarchived Live List of the record type, whoever
// may find it, and keeps the ones whose filter names field. A stored tree that
// no longer decodes names no field.
func liveListsNaming(ctx context.Context, tx pgx.Tx, entityType, field string) ([]listRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.id, l.name, l.sharing, l.definition FROM list l
		WHERE l.list_type = @list_type AND l.entity_type = @entity_type AND l.archived_at IS NULL
		ORDER BY l.name, l.id`,
		pgx.StrictNamedArgs{listTypeField: listTypeDynamic, entityTypeField: entityType})
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (listRow, error) {
		var l listRow
		err := row.Scan(&l.ID, &l.Name, &l.Sharing, &l.Definition)
		return l, err
	})
	if err != nil {
		return nil, err
	}
	var out []listRow
	for _, l := range all {
		pred, err := predicateFromDefinition(l.Definition)
		if err == nil && slices.Contains(storekit.FieldsNamed(pred), field) {
			out = append(out, l)
		}
	}
	return out, nil
}

// findableAmong is which of these lists the caller may find: none without the
// list read grant, else the ones the sharing rule admits.
func findableAmong(ctx context.Context, tx pgx.Tx, lists []listRow) (map[ids.ListID]bool, error) {
	out := map[ids.ListID]bool{}
	switch err := auth.Require(ctx, listObject, principal.ActionRead); {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return out, nil
	case err != nil:
		return nil, err
	}
	listIDs := make([]ids.ListID, 0, len(lists))
	for _, l := range lists {
		listIDs = append(listIDs, l.ID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := fmt.Sprintf("l.id = ANY($%d)", arg(listIDs))
	scope, err := auth.ScopeClauseFor(ctx, listObject, "l", arg)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		where += " AND " + scope
	}
	rows, err := tx.Query(ctx, "SELECT l.id FROM list l WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.ListID])
	for _, id := range found {
		out[id] = true
	}
	return out, err
}
