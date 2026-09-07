// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The write arm as a CLAUSE, for a list that has to say whether the caller
// could act on each row rather than probe one row at a time.
//
// Beside writescope.go's probes rather than inside them: a probe answers about
// one row and refuses, a clause answers about every row and reports. They share
// the predicate underneath, and that sharing is what keeps a list's answer and
// the write's own refusal from drifting apart.

import (
	"context"
	"fmt"
)

// WriteAuthorityClauseFor renders the write arm as a CLAUSE, for a list that
// has to say whether the caller could act on each row rather than probe one row
// at a time — ScopeClauseFor's write-level twin.
//
// It is the write half ONLY. The visibility half is ScopeClauseFor's, and a
// caller asking "may this seat change this row" owes both: EnsureWritable runs
// the visibility probe first for a reason its own comment gives, and a clause
// that carried only this one would answer yes for a row the caller cannot even
// read. Composing them is the caller's job because a list already has the
// visibility clause in hand and would otherwise apply it twice.
//
// Empty means "no narrowing" — an unbounded caller, or a table nothing can be
// shared on — which the caller reads the way ScopeClauseFor's empty is read.
func WriteAuthorityClauseFor(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	if !ownerScopedTables[table] {
		return "", fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if Unbounded(p) || !shareableTables[table] {
		return "", nil
	}
	if alias == "" {
		alias = table
	}
	return writeAuthorityPredicateAs(p, table, alias, arg), nil
}
