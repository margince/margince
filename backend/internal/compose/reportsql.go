// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The report query text: rendering a validated plan into SQL and shaping the
// rows that come back. Every identifier here is a fixed expression the
// report's spec supplied and every caller value travels as a bind parameter,
// which is why this stays the ONE place report SQL is assembled — a second
// builder is how a typed plan quietly becomes free SQL again.

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sqlUnnarrowed is the clause an unbounded reader's scope renders to where a
// predicate is syntactically required: every row passes.
const sqlUnnarrowed = "TRUE"

// reportRowLimit bounds any report; aggregates past this are a data
// export, not a report.
const reportRowLimit = 1000

// fetchRows assembles the WHERE side (validated filters + the caller's
// row-scope clause), runs the plan inside the workspace-bound
// transaction, and shapes each row for the wire.
// reportFrame is what a number needs to be placed: which zone cut the day,
// which currency the money is in, and where the financial year opens. Read in
// the SAME transaction as the rows, so a settings change mid-flight cannot
// hand back a total computed under one frame and labelled with another.
type reportFrame struct {
	Timezone             string
	BaseCurrency         string
	FiscalYearStartMonth int
	// AsOf is the ONE instant this answer is true at, read from the database
	// so every figure in it converts at the same moment.
	//
	// Read here rather than taken from a clock at each expression: two
	// conversions a few milliseconds apart can straddle a rate sheet's
	// effective date, and the stage totals would then sum to a headline
	// nobody can reproduce. It is also what the answer is LABELLED with, so
	// the label and the arithmetic cannot disagree.
	//
	// Held by: TestAReportIsLabelledWithTheInstantItWasComputedAt
	// (backend/gates/reportasof_test.go)
	AsOf time.Time
}

// readReportFrame reads the three installation settings that place a number.
//
// Called ONCE per transaction and threaded to everything that needs them, so
// the values bound into a statement are the same ones its answer is labelled
// with. Reading them again further down would be a second read under READ
// COMMITTED, where a concurrent settings write can land in between.
func readReportFrame(ctx context.Context, tx pgx.Tx) (reportFrame, error) {
	var frame reportFrame
	var err error
	if frame.Timezone, err = identity.TimezoneOf(ctx, tx); err != nil {
		return reportFrame{}, err
	}
	if frame.BaseCurrency, err = identity.BaseCurrencyOf(ctx, tx); err != nil {
		return reportFrame{}, err
	}
	if frame.FiscalYearStartMonth, err = identity.FiscalYearStartMonthOf(ctx, tx); err != nil {
		return reportFrame{}, err
	}
	// The transaction's own start, not the wall clock: inside one READ
	// COMMITTED transaction this is fixed, so every statement assembled from
	// this frame converts at one instant even when they run milliseconds apart.
	if err = tx.QueryRow(ctx, `SELECT now()`).Scan(&frame.AsOf); err != nil {
		return reportFrame{}, fmt.Errorf("compose: reading the report's as-of: %w", err)
	}
	return frame, nil
}

func (e *reportEngine) fetchRows(ctx context.Context, report string, spec reportSpec, req reportRequest, groupBy, selects, columns []string) ([]map[string]any, *int, reportFrame, error) {
	var rows []map[string]any
	var excluded *int
	var frame reportFrame
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		if frame, err = readReportFrame(ctx, tx); err != nil {
			return err
		}
		if err := requireFilterScopes(ctx, tx, spec, req.Filters); err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		where, err := buildReportWhere(ctx, tx, spec, req, callersOwnPopulation(), arg)
		if err != nil {
			return err
		}
		// Field masks exclude their rows from the whole statement — the
		// aggregate and the drill-through must keep reading the identical
		// row set (reportmask.go) — and the withheld count rides the
		// envelope as excluded_by_permission.
		maskClauses, masked, err := maskExclusionClauses(ctx, spec, arg)
		if err != nil {
			return err
		}
		if masked {
			n, err := countMaskExcluded(ctx, tx, frame, spec, where, maskClauses, args)
			if err != nil {
				return err
			}
			excluded = &n
			where = append(where, maskClauses...)
		}
		sql, args, err := bindReportTokens(ctx, frame, reportSQL(spec, selects, where, groupBy), args)
		if err != nil {
			return err
		}
		pgRows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("report %s: %w", report, err)
		}
		defer pgRows.Close()
		rows, err = scanReportRows(pgRows, columns)
		return err
	})
	if err != nil {
		return nil, nil, reportFrame{}, err
	}
	return rows, excluded, frame, nil
}

// reportSQL renders the aggregate query: the validated SELECT list over the
// spec's FROM and WHERE, grouped and ordered by the dimension positions,
// bounded by the report row limit.
func reportSQL(spec reportSpec, selects, where, groupBy []string) string {
	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selects, ", "), spec.fromClause())
	// A report can legitimately restrict NOTHING: leads-by-status counts every
	// lead whatever its status, and an admin's row scope adds no clause of its
	// own. A bare WHERE would then be a syntax error, so the keyword is written
	// only when there is something to write after it.
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if len(groupBy) > 0 {
		positions := make([]string, len(groupBy))
		for i := range groupBy {
			positions[i] = fmt.Sprint(i + 1)
		}
		order := strings.Join(positions, ", ")
		if spec.orderBy != "" {
			order = spec.orderBy + ", " + order
		}
		sql += " GROUP BY " + strings.Join(positions, ", ") + " ORDER BY " + order
	}
	sql += fmt.Sprintf(" LIMIT %d", reportRowLimit)
	return sql
}

// scanReportRows shapes each result row into a column→value map, rendering
// values wire-friendly.
func scanReportRows(pgRows pgx.Rows, columns []string) ([]map[string]any, error) {
	// Empty, never nil. "No deals in that stage" is a real answer and arrives
	// shaped like the array it is: nil marshals to `null`, which a model reads as
	// "unknown". Normalized here so no transport can put null on the wire —
	// reportOutcome.Rows is marshalled straight through on the tool surface.
	rows := []map[string]any{}
	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = wireValue(values[i])
		}
		rows = append(rows, row)
	}
	return rows, pgRows.Err()
}

// wireValue renders driver-native values JSON-friendly: uuids as their
// canonical string, not a 16-byte array.
//
//craft:ignore naked-any report rows are schemaless by design — values arrive driver-native and leave JSON-wire-shaped
func wireValue(v any) any {
	if raw, ok := v.([16]byte); ok {
		return ids.UUID(raw).String()
	}
	return v
}

// quoteIdent admits caller-chosen aggregate aliases into the SQL text safely:
// strict identifier shape, or a fixed literal that cannot carry an injection.
//
// The fallback is safe to reuse across two aggregates in one plan, which looks
// wrong and is not: the SQL alias is never read back. scanReportRows maps each
// value by POSITION into the caller-facing column names built alongside these
// selects, so two aggregates that both fall back still arrive under their own
// distinct names.
func quoteIdent(name string) string {
	if !identShape.MatchString(name) {
		return "result"
	}
	return name
}

var identShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Tokens a spec expression carries in place of a bind position it cannot
// name: the catalog is a static map of expressions, so a value resolved per
// request — the installation's zone or base currency, the caller's scope over
// a table a subquery reads — is written as a token and bound once the
// statement is assembled. None is valid SQL, so a statement that reaches
// Postgres with one unsubstituted fails loudly rather than quietly reporting
// in the wrong zone, the wrong currency, or past the caller's scope.
const (
	// reportZoneToken stands in for the installation timezone's bind position.
	reportZoneToken = "<<installation-timezone>>"
	// reportBaseCurrencyToken stands in for the installation base currency's
	// bind position.
	reportBaseCurrencyToken = "<<installation-base-currency>>"
	// reportFiscalStartMonthToken stands in for the month the installation's
	// business year begins — the value that decides whether a period bucket
	// reads as a calendar year or a fiscal one, and by how much the anchor
	// shifts when it is the latter.
	reportFiscalStartMonthToken = "<<installation-fiscal-start-month>>"
	// reportDealScopeToken stands in for the caller's deal row-scope clause
	// over a deal subquery aliased d — what keeps a per-project money total
	// from disclosing a deal the same caller's deal list would withhold.
	reportDealScopeToken = "<<deal-scope:d>>" //nolint:gosec // an SQL placeholder the engine substitutes, not a credential
	// reportActivityScopeToken stands in for the caller's activity content
	// clause over an activity subquery aliased a, for the same reason.
	reportActivityScopeToken = "<<activity-scope:a>>"
	// reportAsOfToken stands in for the instant the answer is true at — the
	// date an open deal's exchange rate is looked up on or before.
	reportAsOfToken = "<<report-as-of>>" //nolint:gosec // an SQL placeholder the engine substitutes, not a credential
)

// bindReportTokens substitutes every token the assembled statement carries
// for a real bind position, appending the resolved values to THIS statement's
// arguments.
//
// It takes and returns the argument slice rather than sharing one, because a
// caller may assemble several statements from one plan and only some of them
// mention a token: Postgres rejects a parameter the statement never
// references, so a shared slice would break the queries that do not use it.
//
// The frame comes in already resolved rather than being read here, so the
// values bound into the statement are the SAME ones the result is labelled
// with. Reading the settings twice inside one READ COMMITTED transaction lets
// a concurrent settings write land between them, and the answer then reports
// a total cut in one zone under the name of another.
func bindReportTokens(
	ctx context.Context, frame reportFrame, sql string, args []any,
) (string, []any, error) {
	args = slices.Clone(args)
	arg := func(v any) int { args = append(args, v); return len(args) }
	// `any` rather than `string`, because the fiscal start month is a number
	// the statement both compares (= 1) and does arithmetic with. Rendering it
	// as text and letting Postgres infer the type back is the kind of shortcut
	// that works until a comparison silently becomes a text one.
	bindValue := func(token string, value any) {
		if !strings.Contains(sql, token) {
			return
		}
		sql = strings.ReplaceAll(sql, token, fmt.Sprintf("$%d", arg(value)))
	}
	bindValue(reportZoneToken, frame.Timezone)
	bindValue(reportBaseCurrencyToken, frame.BaseCurrency)
	bindValue(reportFiscalStartMonthToken, frame.FiscalYearStartMonth)
	bindValue(reportAsOfToken, frame.AsOf)
	bindScope := func(token string, resolve func() (string, error)) error {
		if !strings.Contains(sql, token) {
			return nil
		}
		clause, err := resolve()
		if err != nil {
			return err
		}
		if clause == "" {
			// An unbounded reader of that table: nothing to narrow.
			clause = sqlUnnarrowed
		}
		sql = strings.ReplaceAll(sql, token, clause)
		return nil
	}
	if err := bindScope(reportDealScopeToken, func() (string, error) { return auth.ScopeClauseFor(ctx, tableDeal, "d", arg) }); err != nil {
		return "", nil, err
	}
	if err := bindScope(reportActivityScopeToken, func() (string, error) { return auth.ActivityContentClause(ctx, "a", arg) }); err != nil {
		return "", nil, err
	}
	return sql, args, nil
}
