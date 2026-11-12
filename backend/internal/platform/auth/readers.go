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
	for _, reader := range readerContexts(ctx, readers) {
		if err := Require(reader, object, principal.ActionRead); err != nil {
			return err
		}
	}
	return nil
}

// ScopeClauseForReaders intersects row visibility without changing the caller.
func ScopeClauseForReaders(ctx context.Context, table, alias string, arg func(any) int, readers ...principal.Principal) (string, error) {
	var clauses []string
	for _, reader := range readerContexts(ctx, readers) {
		var clause string
		var err error
		if table == "activity" {
			clause, err = ActivityContentClause(reader, alias, arg)
		} else {
			clause, err = ScopeClauseFor(reader, table, alias, arg)
		}
		if err != nil {
			return "", err
		}
		if clause != "" {
			clauses = append(clauses, "("+clause+")")
		}
	}
	return strings.Join(clauses, " AND "), nil
}

func readerContexts(ctx context.Context, readers []principal.Principal) []context.Context {
	contexts := []context.Context{ctx}
	for _, reader := range readers {
		contexts = append(contexts, principal.WithActor(ctx, reader))
	}
	return contexts
}
