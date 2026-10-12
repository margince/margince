// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Which lists one record is on, as its record page names them: the Shortlists
// it was chosen for and the Live Lists whose filter selects it now. Only lists
// the reader may find, and only for a record they may see.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordListTypes are the record types a record page offers lists on.
var recordListTypes = map[string]bool{typeContact: true, typeCompany: true, typeDeal: true, typeLead: true}

// RecordLists is the lists one record is on that the caller may find, by
// name, and whether more were found than one answer holds.
type RecordLists struct {
	Lists     []crmcontracts.List
	Truncated bool
}

// RecordListsFor reads the lists one record is on that this caller may find.
// A record that does not exist, is archived, or lies outside the caller's row
// scope answers ErrNotFound.
func (s *Store) RecordListsFor(ctx context.Context, entityType string, entityID ids.UUID) (RecordLists, error) {
	if !recordListTypes[entityType] {
		return RecordLists{}, &BadInputError{Field: entityTypeField, Reason: "must be contact, company, deal or lead"}
	}
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return RecordLists{}, err
	}
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return RecordLists{}, err
	}
	// Resolved before the transaction: the engine reads the field catalog on a
	// connection of its own (see liveFilter).
	engine, ok, err := s.SegmentEngine(ctx, entityType)
	if err != nil {
		return RecordLists{}, err
	}
	if !ok {
		return RecordLists{}, fmt.Errorf("no dynamic segment engine for entity_type %q", entityType)
	}
	var on []listRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The live probe asks even an unbounded reader whether the row exists.
		if err := auth.EnsureVisibleLive(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		var readErr error
		on, readErr = listsHolding(ctx, tx, engine, entityType, entityID)
		return readErr
	})
	if err != nil {
		return RecordLists{}, err
	}
	out := RecordLists{Lists: make([]crmcontracts.List, 0, min(len(on), catalogCap))}
	if len(on) > catalogCap {
		on, out.Truncated = on[:catalogCap], true
	}
	for _, l := range on {
		out.Lists = append(out.Lists, foundList(ctx, l))
	}
	return out, nil
}

// listsHolding reads the Shortlists the record was chosen for, up to one past
// the cap, and every findable Live List whose filter selects it, by name.
func listsHolding(ctx context.Context, tx pgx.Tx, engine storekit.Query, entityType string, entityID ids.UUID) ([]listRow, error) {
	chosen, err := findableShortlistsHolding(ctx, tx, entityType, entityID, catalogCap+1)
	if err != nil {
		return nil, err
	}
	live, err := findableLiveLists(ctx, tx, entityType)
	if err != nil {
		return nil, err
	}
	selecting, err := selectingLists(ctx, tx, engine, live, entityID)
	if err != nil {
		return nil, err
	}
	out := slices.Concat(chosen, selecting)
	slices.SortFunc(out, func(a, b listRow) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return out, nil
}

// liveBatch bounds how many filters one statement judges, well inside
// Postgres's limit on the columns a statement may select.
const liveBatch = 200

// selectingLists keeps each Live List whose filter selects the record. A Live
// List whose filter no longer compiles holds nobody, as its member read says.
func selectingLists(ctx context.Context, tx pgx.Tx, engine storekit.Query, live []listRow, entityID ids.UUID) ([]listRow, error) {
	var judged []listRow
	var filters []storekit.Predicate
	for _, l := range live {
		pred, err := predicateFromDefinition(l.Definition)
		if err != nil || !compiles(engine, pred) {
			continue
		}
		judged, filters = append(judged, l), append(filters, pred)
	}
	var out []listRow
	for start := 0; start < len(filters); start += liveBatch {
		end := min(start+liveBatch, len(filters))
		selected, err := engine.SelectsEach(ctx, tx, filters[start:end], entityID)
		if err != nil {
			return nil, err
		}
		for i, on := range selected {
			if on {
				out = append(out, judged[start+i])
			}
		}
	}
	return out, nil
}

// compiles says whether a stored filter still compiles against the engine's
// vocabulary; one naming a field since removed does not.
func compiles(engine storekit.Query, pred storekit.Predicate) bool {
	discard := 0
	_, err := storekit.CompilePredicate(pred, engine.Fields, func(any) int { discard++; return discard })
	return err == nil
}

// findableShortlistsHolding reads the live Shortlists of entityType the caller
// may find that the record was chosen for, by name, at most limit of them.
func findableShortlistsHolding(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, limit int) ([]listRow, error) {
	return findableLists(ctx, tx, entityType, &limit, func(typePos int, arg func(any) int) string {
		return fmt.Sprintf(`l.list_type = '%s' AND EXISTS (SELECT 1 FROM list_member m
			WHERE m.list_id = l.id AND m.entity_type = $%d AND m.entity_id = $%d)`, listTypeStatic, typePos, arg(entityID))
	})
}

// findableLiveLists reads every live Live List of entityType the caller may
// find. Unbounded: each is judged, and a cap here would drop a list that
// selects the record without saying so.
func findableLiveLists(ctx context.Context, tx pgx.Tx, entityType string) ([]listRow, error) {
	return findableLists(ctx, tx, entityType, nil, func(int, func(any) int) string {
		return fmt.Sprintf("l.list_type = '%s'", listTypeDynamic)
	})
}

// findableLists reads the unarchived lists of entityType the caller may find
// that narrow selects, by name, at most limit of them when limit is set.
func findableLists(ctx context.Context, tx pgx.Tx, entityType string, limit *int, narrow func(typePos int, arg func(any) int) string) ([]listRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos := arg(entityType)
	where := narrow(typePos, arg)
	scope, err := recordScope(ctx, listObject, "l", arg)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(`SELECT %s FROM list l
		WHERE l.entity_type = $%d AND l.archived_at IS NULL AND %s AND %s
		ORDER BY l.name, l.id`, listColumns, typePos, where, scope)
	if limit != nil {
		sql += fmt.Sprintf(" LIMIT $%d", arg(*limit))
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (listRow, error) { return scanList(row) })
}

// ShortlistsHolding reads the Shortlists one record was chosen for that the
// caller may find, by name, at most limit of them, inside the caller's
// transaction. The caller has already established it may see the record.
func ShortlistsHolding(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, limit int) ([]crmcontracts.List, error) {
	lists, err := findableShortlistsHolding(ctx, tx, entityType, entityID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.List, 0, len(lists))
	for _, l := range lists {
		out = append(out, foundList(ctx, l))
	}
	return out, nil
}

// foundList is a list as a membership answer names it: its identity, kind and
// sharing, without the count and health a list read judges.
func foundList(ctx context.Context, l listRow) crmcontracts.List {
	return wireList(listSummary{listRow: l, Health: healthOK, CanEdit: mayEditList(ctx, l)})
}
