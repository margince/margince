// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The one-engine EXECUTOR: a resource's compiled predicate, run as bounded SQL
// inside the caller's workspace-bound transaction.
//
// Split from predicate.go, which compiles a filter and nothing else. That
// separation is the point rather than a filing convenience: compilation is
// scope-neutral by design, and every guard that makes a compiled filter SAFE to
// run — the object-RBAC admission, the row-scope clause, the row bound — lives
// on this side, in one place a surface cannot route around.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Query is the one-engine executor (B-E15.10b): one resource's closed
// vocabulary plus its fixed base clause, executing any Predicate as
// bounded, indexable SQL over the real columns. Lists, saved views, and
// filtered export all run their filters through here — never through a
// per-surface variant.
type Query struct {
	// Table is the base table, aliased t in every Fields expression.
	Table string
	// Fields is the resource's §13.5 filter allow-list.
	Fields map[string]Field
	// BaseWhere is the resource's fixed visibility clause (e.g.
	// "t.archived_at IS NULL"); empty means none.
	BaseWhere string
	// ActivityWalk selects the activity link-walk scope clause instead
	// of the direct ownership clause (the timeline's visibility rule).
	ActivityWalk bool
}

// predicateWhere is the admission point AND the clause composer for every
// execution of a predicate: it takes the object read gate, then joins the
// resource's base clause, the compiled predicate, and the caller's row scope.
//
// Both live here so a second executor cannot take two of the three, or the
// clauses without the gate. The row scope is the one that matters most: it is
// what makes a predicate able only to NARROW what the caller may already see,
// and an executor that forgot it would still return plausible rows, just other
// contacts's. One point of composition means "is this scoped?" has one answer for
// every caller.
func (q Query) predicateWhere(ctx context.Context, p Predicate) (string, []any, error) {
	return q.scopedWhere(ctx, func(arg func(any) int) (string, error) {
		return CompilePredicate(p, q.Fields, arg)
	})
}

// scopedWhere is predicateWhere for any narrowing clause: the gate, the base
// clause, the narrowing and the row scope, composed in the one place.
func (q Query) scopedWhere(ctx context.Context, narrow func(arg func(any) int) (string, error)) (string, []any, error) {
	if err := auth.Require(ctx, q.Table, principal.ActionRead); err != nil {
		return "", nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	where := make([]string, 0, 3)
	if q.BaseWhere != "" {
		where = append(where, q.BaseWhere)
	}
	compiled, err := narrow(arg)
	if err != nil {
		return "", nil, err
	}
	where = append(where, compiled)

	var scope string
	if q.ActivityWalk {
		scope, err = auth.ActivityContentClause(ctx, "t", arg)
	} else {
		scope, err = auth.ScopeClauseFor(ctx, q.Table, "t", arg)
	}
	if err != nil {
		return "", nil, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	return strings.Join(where, " AND "), args, nil
}

// CountMatching answers how many rows the predicate selects for this caller.
//
// It is a COUNT rather than len(SelectIDs): SelectIDs is capped at
// PredicateRowLimit, so counting its result would silently report 1000 for every
// larger set — a number that looks exact and is not. The filter builder shows
// this figure to a human deciding whether their filter is right, which is
// precisely the situation where a plausible wrong number is worse than a slow
// one.
//
// Unbounded on purpose, and it is the same WHERE SelectIDs runs: same base
// clause, same predicate, same row scope, so the count and the page it labels
// cannot disagree about what MATCHING MEANS. Whether they saw the same rows is a
// question about the snapshot, not the clause, and the caller decides that by
// choosing which transaction to run both in.
func (q Query) CountMatching(ctx context.Context, tx pgx.Tx, p Predicate) (int, error) {
	where, args, err := q.predicateWhere(ctx, p)
	if err != nil {
		return 0, err
	}
	var n int
	sql := fmt.Sprintf("SELECT count(*) FROM %s t WHERE %s", q.Table, where)
	if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("predicate count on %s: %w", q.Table, err)
	}
	return n, nil
}

// SelectIDs runs the predicate inside the caller's workspace-bound
// transaction and returns matching row ids, deterministically ordered
// by id (the keyset tie-breaker) and hard-capped at PredicateRowLimit.
//
// The read gate and the row-scope clause both come from predicateWhere — see
// there for why they live together — so a predicate can only ever narrow what
// the caller is already allowed to see. The cap stays with the select rather than in
// predicateWhere: it bounds a PAGE, and a count must not inherit it.
func (q Query) SelectIDs(ctx context.Context, tx pgx.Tx, p Predicate, limit int) ([]ids.UUID, error) {
	where, args, err := q.predicateWhere(ctx, p)
	if err != nil {
		return nil, err
	}
	return q.selectCapped(ctx, tx, where, args, limit)
}

// SelectMemberIDs is SelectIDs over a list's members instead of a predicate:
// the same gate, base clause, row scope and cap, narrowed by members.
func (q Query) SelectMemberIDs(ctx context.Context, tx pgx.Tx, members ListMemberFilter, limit int) ([]ids.UUID, error) {
	where, args, err := q.scopedWhere(ctx, func(arg func(any) int) (string, error) {
		return members("t.id", arg)
	})
	if err != nil {
		return nil, err
	}
	return q.selectCapped(ctx, tx, where, args, limit)
}

func (q Query) selectCapped(ctx context.Context, tx pgx.Tx, where string, args []any, limit int) ([]ids.UUID, error) {
	if limit <= 0 || limit > PredicateRowLimit {
		limit = PredicateRowLimit
	}
	sql := fmt.Sprintf("SELECT t.id FROM %s t WHERE %s ORDER BY t.id LIMIT %d",
		q.Table, where, limit)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("predicate query on %s: %w", q.Table, err)
	}
	defer rows.Close()

	var matched []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("predicate query on %s: %w", q.Table, err)
		}
		matched = append(matched, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("predicate query on %s: %w", q.Table, err)
	}
	return matched, nil
}

// SelectPage runs the predicate as one keyset page: up to limit matching ids
// after `after` (nil for the first page), id-ordered, and whether more follow.
// Unlike SelectIDs it has no ceiling on the whole set, so a caller can walk
// every match a page at a time and a set past PredicateRowLimit is complete.
func (q Query) SelectPage(ctx context.Context, tx pgx.Tx, p Predicate, after *ids.UUID, limit int) ([]ids.UUID, bool, error) {
	limit = ClampLimit(&limit)
	where, args, err := q.predicateWhere(ctx, p)
	if err != nil {
		return nil, false, err
	}
	if after != nil {
		args = append(args, *after)
		where += fmt.Sprintf(" AND t.id > $%d", len(args))
	}
	args = append(args, limit+1)
	sql := fmt.Sprintf("SELECT t.id FROM %s t WHERE %s ORDER BY t.id LIMIT $%d", q.Table, where, len(args))
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, fmt.Errorf("predicate page on %s: %w", q.Table, err)
	}
	matched, err := ScanUUIDColumn(rows, "predicate page on "+q.Table)
	if err != nil {
		return nil, false, err
	}
	if len(matched) > limit {
		return matched[:limit], true, nil
	}
	return matched, false, nil
}

// MatchClause renders "this row is selected by the predicate" for a caller's
// own statement over the same table, whose id column is idColumn. It carries
// the resource's base clause and the compiled filter and NO row scope: the
// caller's statement is a read that already applies its own, and this only
// narrows it.
func (q Query) MatchClause(p Predicate, idColumn string, arg func(any) int) (string, error) {
	compiled, err := CompilePredicate(p, q.Fields, arg)
	if err != nil {
		return "", err
	}
	where := compiled
	if q.BaseWhere != "" {
		where = q.BaseWhere + " AND " + compiled
	}
	return fmt.Sprintf("%s IN (SELECT t.id FROM %s t WHERE %s)", idColumn, q.Table, where), nil
}
