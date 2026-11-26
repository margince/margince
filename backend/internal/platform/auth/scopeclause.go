// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The row predicates rendered as a CLAUSE for a caller's own query, rather than
// probed one row at a time.
//
// A list, a search and a report each hold their own statement and compose the
// clause into it; the probes next door answer about one row and refuse. They
// share the predicate underneath (rowscope.go), which is what keeps a list's
// answer and a probe's refusal from disagreeing about the same row.

import (
	"context"
	"fmt"
)

// ScopeClause renders the own/team/all row-visibility predicate over an
// owner_id column (B-EP03.3a). arg registers a query argument and
// returns its 1-based position, matching the list builders' convention.
// An empty clause means unbounded (row_scope=all, or the system actor).
// Ownerless rows (owner_id IS NULL) are workspace-shared and visible at
// every tier.
func ScopeClause(ctx context.Context, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if Unbounded(p) {
		return "", nil
	}
	return OwnerPredicate(p, arg)(""), nil
}

// writeShareClauseFor is ScopeClauseFor with the share arm narrowed to a
// `write` grant, the visibility half of AttachClauseFor. Empty means no
// narrowing, as ScopeClauseFor's does.
func writeShareClauseFor(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	return clauseFor(ctx, table, alias, arg, writeShare)
}

// ScopeClauseFor renders the full visibility predicate (owner scope OR
// live record grant) for one named table with an alias — the spelling
// every list/search/report path over a shareable table uses.
func ScopeClauseFor(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	return clauseFor(ctx, table, alias, arg, anyShare)
}

// clauseFor is the body both spellings share: the table check, the unbounded
// shortcut and the predicate call, written where the read direction and the
// attach one read the same one.
func clauseFor(ctx context.Context, table, alias string, arg func(any) int, share shareLevel) (string, error) {
	if !ownerScopedTables[table] {
		return "", fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if UnboundedFor(p, table) {
		return "", nil
	}
	return predicateFor(p, table, arg, withCapturePrivacy, asClassified, share)(alias), nil
}

// DealChildScopeClause renders the row scope of a record that has no owner and
// is seen when its deal is. The room list, the single room read, search and the
// retirement preview all render it, so they show a caller the same rooms.
//
// The deal is an `EXISTS` and not a `JOIN`. That keeps one `id` and one
// `created_at` in the caller's keyset. An unbounded caller still gets a clause.
func DealChildScopeClause(ctx context.Context, childAlias string, arg func(any) int) (string, error) {
	scope, err := ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = truePredicate
	}
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM deal d WHERE d.id = %s.deal_id AND (%s))`, childAlias, scope), nil
}
