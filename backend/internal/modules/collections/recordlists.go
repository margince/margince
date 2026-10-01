// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Which lists one record is on, as its record page names them: the Shortlists
// it was chosen for and the Live Lists whose filter selects it now. Only lists
// the reader may find, and only for a record they may see.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordListTypes are the record types a record page offers lists on.
var recordListTypes = map[string]bool{typeContact: true, typeCompany: true, typeDeal: true, typeLead: true}

// RecordListsFor reads the lists one record is on that this caller may find,
// by name. A record outside the caller's row scope answers ErrNotFound.
func (s *Store) RecordListsFor(ctx context.Context, entityType string, entityID ids.UUID) ([]crmcontracts.List, error) {
	if !recordListTypes[entityType] {
		return nil, &BadInputError{Field: entityTypeField, Reason: "must be contact, company, deal or lead"}
	}
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return nil, err
	}
	// Resolved before the transaction: the engine reads the field catalog on a
	// connection of its own (see liveFilter).
	engine, ok, err := s.SegmentEngine(ctx, entityType)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no dynamic segment engine for entity_type %q", entityType)
	}
	var on []listRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		candidates, err := findableListsHolding(ctx, tx, entityType, entityID, true)
		if err != nil {
			return err
		}
		on, err = selectedLists(ctx, tx, engine, candidates, entityID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.List, 0, len(on))
	for _, l := range on {
		out = append(out, foundList(ctx, l))
	}
	return out, nil
}

// selectedLists keeps every Shortlist among candidates and each Live List
// whose filter selects the record, judged in one statement. A Live List whose
// filter no longer compiles holds nobody, as its member read would say.
func selectedLists(ctx context.Context, tx pgx.Tx, engine storekit.Query, candidates []listRow, entityID ids.UUID) ([]listRow, error) {
	var live []listRow
	var filters []storekit.Predicate
	for _, l := range candidates {
		if l.ListType != listTypeDynamic {
			continue
		}
		pred, err := predicateFromDefinition(l.Definition)
		if err != nil || !compiles(engine, pred) {
			continue
		}
		live, filters = append(live, l), append(filters, pred)
	}
	selected, err := engine.SelectsEach(ctx, tx, filters, entityID)
	if err != nil {
		return nil, err
	}
	isOn := map[ids.ListID]bool{}
	for i, l := range live {
		isOn[l.ID] = selected[i]
	}
	var out []listRow
	for _, l := range candidates {
		if l.ListType != listTypeDynamic || isOn[l.ID] {
			out = append(out, l)
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

// findableListsHolding reads the live lists of entityType the caller may find
// that may hold the record, by name: each Shortlist it was chosen for and,
// withLive, every Live List, whose filter the caller still has to judge.
func findableListsHolding(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, withLive bool) ([]listRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos := arg(entityType)
	chosen := fmt.Sprintf(`l.list_type = '%s' AND EXISTS (SELECT 1 FROM list_member m
		WHERE m.list_id = l.id AND m.entity_type = $%d AND m.entity_id = $%d)`, listTypeStatic, typePos, arg(entityID))
	if withLive {
		chosen = fmt.Sprintf("(l.list_type = '%s' OR (%s))", listTypeDynamic, chosen)
	}
	scope, err := recordScope(ctx, listObject, "l", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM list l
		WHERE l.entity_type = $%d AND l.archived_at IS NULL AND %s AND %s
		ORDER BY l.name, l.id LIMIT $%d`, listColumns, typePos, chosen, scope, arg(catalogCap)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (listRow, error) { return scanList(row) })
}

// ShortlistsHolding reads the Shortlists one record was chosen for that the
// caller may find, by name, at most limit of them, inside the caller's
// transaction. The caller has already established it may see the record.
func ShortlistsHolding(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, limit int) ([]crmcontracts.List, error) {
	lists, err := findableListsHolding(ctx, tx, entityType, entityID, false)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.List, 0, min(len(lists), limit))
	for _, l := range lists[:min(len(lists), limit)] {
		out = append(out, foundList(ctx, l))
	}
	return out, nil
}

// foundList is a list as a membership answer names it: its identity, kind and
// sharing, without the count and health a list read judges.
func foundList(ctx context.Context, l listRow) crmcontracts.List {
	return wireList(listSummary{listRow: l, Health: healthOK, CanEdit: mayEditList(ctx, l)})
}
