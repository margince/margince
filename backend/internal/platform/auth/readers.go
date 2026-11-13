// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequireReadForReaders checks every principal whose access bounds a shared read.
func RequireReadForReaders(ctx context.Context, object string, readers ...principal.Principal) error {
	return eachReader(ctx, readers, func(reader context.Context) error {
		return Require(reader, object, principal.ActionRead)
	})
}

// ScopeClauseForReaders intersects row visibility without changing the caller.
func ScopeClauseForReaders(ctx context.Context, table, alias string, arg func(any) int, readers ...principal.Principal) (string, error) {
	var clauses []string
	err := eachReader(ctx, readers, func(reader context.Context) error {
		clause, err := readerScopeClause(reader, table, alias, arg)
		if err != nil {
			return err
		}
		if clause != "" {
			clauses = append(clauses, "("+clause+")")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return strings.Join(clauses, " AND "), nil
}

// activityObject is the table whose reads also go through the content clause.
const activityObject = "activity"

// readerScopeClause bounds activities by the content clause, which also holds
// the mail audience, and every other table by its row scope.
func readerScopeClause(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	if table == activityObject {
		return ActivityContentClause(ctx, alias, arg)
	}
	return ScopeClauseFor(ctx, table, alias, arg)
}

// eachReader runs check as the caller, then as each reader, and stops at the
// first refusal.
func eachReader(ctx context.Context, readers []principal.Principal, check func(context.Context) error) error {
	if err := check(ctx); err != nil {
		return err
	}
	for _, reader := range readers {
		if err := check(principal.WithActor(ctx, reader)); err != nil {
			return err
		}
	}
	return nil
}
