// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// countQuery is one prepared COUNT(*): the statement and its bind values.
type countQuery struct {
	sql  string
	args []any
}

// countWhere builds the COUNT(*) that labels a first page: how many rows match
// the request. It runs only on a page without a cursor, so it counts the whole set.
//
// It re-runs the spec's OWN filters closure against a fresh argument counter
// rather than reusing the page's clauses, so the count is assembled by the same
// code as the page and a filter cannot reach one without reaching the other.
//
// The page's already-parsed sort is passed in rather than re-parsed. A
// reference sort (employer name, say) renders an expression that BINDS
// parameters, and re-parsing here would register them on this statement's
// argument slice while nothing in a COUNT's WHERE ever uses them — Postgres
// then refuses the statement outright, because a bind with no placeholder has
// no type to infer ("could not determine data type of parameter $1"). The
// count needs the sort only for the filters signature. The one clause that
// reads it is the keyset cursor, and a first page has none.
//
// Held by: TestTheContactsListSortsByTheEmployerItDraws and
// TestAnUnreadableEmployerOrdersTheContactsListByNothing
// (backend/internal/compose/integration/listreferencesorts_integration_test.go),
// which sort by a drawn column and fail on the orphaned bind.
//
// Held by: TestAListTotalCountsTheFilteredSet and
// TestAListTotalCountsOnlyTheRowsTheReaderMaySee
// (backend/internal/modules/contacts/listtotal_integration_test.go): one fails
// if the request's filters stop reaching the count, the other if the row scope
// does.
func countWhere[T any](
	ctx context.Context,
	spec listPageSpec[T],
	active []fieldcatalog.Column,
	sorted *storekit.ListSort,
) (countQuery, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	where := []string{whereAlways}
	// The caller's row scope, asked again on this statement's own counter. A
	// count that skipped it would say how many rows EXIST rather than how many
	// this reader may see, which is a disclosure wearing a total's clothes.
	scope, err := auth.ScopeClauseFor(ctx, spec.entity, "", arg)
	if err != nil {
		return countQuery{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}

	filters, err := spec.filters(ctx, active, sorted, arg)
	if err != nil {
		return countQuery{}, err
	}
	where = append(where, filters...)

	return countQuery{
		sql:  `SELECT count(*) FROM ` + spec.entity + ` WHERE ` + strings.Join(where, " AND "),
		args: args,
	}, nil
}

// firstPageTotal labels a first page. A later page sends no total, because the
// client keeps the first page's and a recount per page reads the whole set again.
// fetched includes the extra row requested to detect another page.
func firstPageTotal(ctx context.Context, tx pgx.Tx, total countQuery, fetched, limit int) (int, error) {
	if fetched <= limit {
		return fetched, nil
	}
	var matching int
	err := tx.QueryRow(ctx, total.sql, total.args...).Scan(&matching)
	return matching, err
}
