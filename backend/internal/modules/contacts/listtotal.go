// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The total beside a list page: how many rows match the request once the
// keyset cursor is set aside. Split from listpage.go, which owns the page
// read — the count is a second statement over the same WHERE, and keeping it
// here is what makes "same filters, same scope, no cursor" one readable rule
// rather than a branch inside the page read.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// countPassKey marks the context of the count pass, so the shared filter
// chain drops the keyset cursor and counts the whole matching set.
//
// A context value rather than a parameter because the three record lists build
// their own listFilters INSIDE the spec's filters closure — contact_list.go,
// company_list.go and lead_list.go each spell one — and the count re-runs that
// same closure so a filter can never reach the page without reaching the
// count. Threading a flag through would mean editing every closure to forward
// it: three chances to forward it wrong today, and a fourth the next record
// list forgets, with the failure being a total that silently counts the wrong
// set.
//
// Held by: TestAListTotalDoesNotShrinkAsTheReaderPagesForward
// (backend/internal/modules/contacts/listtotal_integration_test.go), which
// pages forward and fails if the count inherited the cursor — the one thing
// this mark exists to prevent.
type countPassKey struct{}

// countingSet reports whether ctx is the count pass rather than the page read.
func countingSet(ctx context.Context) bool {
	return ctx.Value(countPassKey{}) != nil
}

// forCountPass marks ctx as the count pass.
func forCountPass(ctx context.Context) context.Context {
	return context.WithValue(ctx, countPassKey{}, struct{}{})
}

// countQuery is one prepared COUNT(*): the statement and its bind values.
type countQuery struct {
	sql  string
	args []any
}

// countWhere builds the COUNT(*) that labels a page — how many rows match the
// request once the cursor is set aside.
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
// count needs the sort only to satisfy the filters signature: the one clause
// that reads it is the keyset cursor, which the count pass drops.
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

	counting := forCountPass(ctx)
	where := []string{whereAlways}
	// The caller's row scope, asked again on this statement's own counter. A
	// count that skipped it would say how many rows EXIST rather than how many
	// this reader may see, which is a disclosure wearing a total's clothes.
	scope, err := auth.ScopeClauseFor(counting, spec.entity, "", arg)
	if err != nil {
		return countQuery{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}

	filters, err := spec.filters(counting, active, sorted, arg)
	if err != nil {
		return countQuery{}, err
	}
	where = append(where, filters...)

	return countQuery{
		sql:  `SELECT count(*) FROM ` + spec.entity + ` WHERE ` + strings.Join(where, " AND "),
		args: args,
	}, nil
}
