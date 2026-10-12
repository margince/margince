// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Whether a reader can open the record a stage-evidence row was read from.
//
// The source pair has no foreign key, by decision. The observation outlives
// its source, so a hard delete of a cited activity or contract goes through.
// A source this reader may not see answers as a deleted one does. So the
// ledger never tells a hidden record from a missing one.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContractVisibleClause renders the predicate admitting the contracts this
// caller may see, for a contract row under alias. `contracts` owns that rule,
// so the edge is injected here (ADR-0054). An empty clause admits every row.
type ContractVisibleClause func(ctx context.Context, alias string, arg func(any) int) (string, error)

// sqlTrue and sqlFalse are the literals a rendered predicate answers with.
const (
	sqlTrue  = "TRUE"
	sqlFalse = "FALSE"
)

func refusingContractVisible() ContractVisibleClause {
	return func(context.Context, string, func(any) int) (string, error) {
		return "", errors.New("deals: the ContractVisible seam was not injected; " +
			"construct this store with installseam.Deals(), which binds modules/contracts to it")
	}
}

// sourceMissingSQL renders, for the evidence row aliased ev, whether the
// caller cannot open its source: deleted, or hidden from them. Each arm uses
// that reader's own gate, the object permission and the row clause.
func (s *Store) sourceMissingSQL(ctx context.Context, arg func(any) int) (string, error) {
	activityRows, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	contractRows, err := s.installation.ContractVisible(ctx, "k", arg)
	if err != nil {
		return "", err
	}
	if contractRows == "" {
		contractRows = sqlTrue
	}
	return storekit.SQLf(`NOT (CASE ev.source_type
	WHEN 'activity' THEN %s AND EXISTS (SELECT 1 FROM activity a WHERE a.id = ev.source_id AND %s)
	WHEN 'contract' THEN %s AND EXISTS (SELECT 1 FROM contract k WHERE k.id = ev.source_id AND %s)
	ELSE false END)`,
		sqlAllows(ctx, "activity"), activityRows, sqlAllows(ctx, "contract"), contractRows), nil
}

// sqlAllows renders the caller's read permission on an object as a literal.
func sqlAllows(ctx context.Context, object string) string {
	if auth.Allows(ctx, object, principal.ActionRead) {
		return sqlTrue
	}
	return sqlFalse
}
