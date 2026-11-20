// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The single-row probes, asked of many rows in one statement each. Each renders
// the clause its single-row twin renders, so the two answer alike for every row.
// Only the 403/404 split is lost, and a subset reader hides both.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RowRule names the single-row probe a subset question asks of each row.
type RowRule int

const (
	// RuleVisibleLive is EnsureVisibleLive.
	RuleVisibleLive RowRule = iota
	// RuleWritableLive is EnsureWritableLive.
	RuleWritableLive
	// RuleWritable is EnsureWritable.
	RuleWritable
)

// PassingSubset answers which of rowIDs pass rule, in one statement. Absent
// means refused, whether the row is missing, archived, out of scope or not the
// caller's to change.
func PassingSubset(ctx context.Context, tx pgx.Tx, table string, rowIDs []ids.UUID, rule RowRule) (map[ids.UUID]bool, error) {
	if !ownerScopedTables[table] {
		return nil, fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idTerm := fmt.Sprintf("id = ANY($%d)", arg(rowIDs))
	visible, err := rowVisibleTerms(ctx, table, arg, rule != RuleWritable)
	if err != nil {
		return nil, err
	}
	terms := append([]string{idTerm}, visible...)
	// ensureWriteAuthority's own shortcuts, so the arm is added where the
	// single-row probe would have queried it.
	if rule != RuleVisibleLive && !Unbounded(p) && shareableTables[table] {
		write := writeAuthorityPredicate(p, table, arg)
		admitted := slices.DeleteFunc(slices.Clone(rowIDs), func(id ids.UUID) bool { return !admittedCaseMember(ctx, table, id) })
		if len(admitted) > 0 {
			write = fmt.Sprintf("(%s OR id = ANY($%d))", write, arg(admitted))
		}
		terms = append(terms, write)
	}
	// EnsureWritable's visibility half skips the query when nothing narrows it.
	if len(terms) == 1 {
		out := make(map[ids.UUID]bool, len(rowIDs))
		for _, id := range rowIDs {
			out[id] = true
		}
		return out, nil
	}
	return idsWhere(ctx, tx, table, "id", terms, args)
}

// ActivityContentSubset is EnsureActivityContentVisible, or its Live twin, over
// many activities.
func ActivityContentSubset(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID, live bool) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	terms := []string{fmt.Sprintf("a.id = ANY($%d)", arg(rowIDs))}
	clause, err := ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if live {
		terms = append(terms, "a.archived_at IS NULL")
	}
	return idsWhere(ctx, tx, "activity a", "a.id", append(terms, clause), args)
}

// SignalVisibleLiveSubset is EnsureSignalVisibleLive over many signals.
func SignalVisibleLiveSubset(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	terms := []string{fmt.Sprintf("s.id = ANY($%d)", arg(rowIDs)), "s.archived_at IS NULL"}
	clause, err := SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return nil, err
	}
	if clause != "" {
		terms = append(terms, clause)
	}
	return idsWhere(ctx, tx, "signal s", "s.id", terms, args)
}

// RelationshipVisibleSubset is EnsureRelationshipVisible over many edges.
func RelationshipVisibleSubset(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	terms := []string{fmt.Sprintf("r.id = ANY($%d)", arg(rowIDs))}
	clause, err := RelationshipEndpointScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if clause != "" {
		terms = append(terms, clause)
	}
	return idsWhere(ctx, tx, "relationship r", "r.id", terms, args)
}

// idsWhere runs the subset statement. from and column are compile-time
// literals of this file; table is checked against ownerScopedTables first.
func idsWhere(ctx context.Context, tx pgx.Tx, from, column string, terms []string, args []any) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s`, column, from, strings.Join(terms, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	passing, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(passing))
	for _, id := range passing {
		out[id] = true
	}
	return out, nil
}
