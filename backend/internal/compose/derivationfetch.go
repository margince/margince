// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The derivation's EXECUTION half: the drill-through rows, the aggregate
// recompute, and the shared WHERE side that keeps them reading the identical
// row set. The compile half (plan validation, definition rendering, the
// handle round-trip) stays in derivation.go.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
)

// fetchDerivation executes a compiled plan: the drill-through rows and
// the aggregate recompute run over the identical WHERE side (validated
// predicates + the caller's row-scope clause) in one transaction.
func (e *reportEngine) fetchDerivation(ctx context.Context, report string, spec reportSpec, plan derivationPlan, out *derivationOutcome) error {
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		if err := requireFilterScopes(ctx, tx, spec, predicatesAsFilters(plan.predicates)); err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }

		where, err := derivationWhere(ctx, tx, spec, plan, callersOwnPopulation(), arg)
		if err != nil {
			return err
		}
		// The identical mask exclusion the report applied (reportmask.go):
		// the explanation must not out-see the number it explains, and the
		// withheld count rides the envelope the same way.
		frame, err := readReportFrame(ctx, tx)
		if err != nil {
			return err
		}
		// The headline's instant wins over this transaction's. Without it a
		// converted report's detail looks up a rate the headline never used,
		// and the two disagree by however much the sheet moved in between.
		if !plan.asOf.IsZero() {
			frame.AsOf = plan.asOf
		}
		// Reported either way, because "which instant" is the question a reader
		// doubting this detail is actually asking.
		out.AsOf = frame.AsOf
		maskClauses, masked, err := maskExclusionClauses(ctx, spec, arg)
		if err != nil {
			return err
		}
		if masked {
			n, err := countMaskExcluded(ctx, tx, frame, spec, where, maskClauses, args)
			if err != nil {
				return err
			}
			out.ExcludedByPermission = &n
			where = append(where, maskClauses...)
		}
		whereSQL := strings.Join(where, " AND ")

		// A reference this reader cannot open comes back NULL, and its row
		// stays. That is what a normal deal read does with the same column
		// (deals/fieldmask.go blanks the reference and keeps the deal), so the
		// drill-through and the list its link opens describe one population.
		//
		// On its OWN argument slice, forked from the shared one. The mask binds
		// the reader's scope values, and only the rows statement names them —
		// Postgres rejects a parameter a statement never references, so letting
		// these land in `args` would break the aggregate recompute below, which
		// selects no reference at all.
		rowArgs := slices.Clone(args)
		rowArg := func(v any) int { rowArgs = append(rowArgs, v); return len(rowArgs) }
		selects, err := maskedDerivationSelects(ctx, spec, plan, rowArg)
		if err != nil {
			return err
		}
		rowsSQL, rowsArgs, err := bindReportTokens(ctx, frame, fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY t.id LIMIT %d",
			strings.Join(selects, ", "), spec.fromClause(), whereSQL, reportRowLimit), rowArgs)
		if err != nil {
			return err
		}
		pgRows, err := tx.Query(ctx, rowsSQL, rowsArgs...)
		if err != nil {
			return fmt.Errorf("derivation %s rows: %w", report, err)
		}
		defer pgRows.Close()
		out.Rows, err = scanDerivationRows(pgRows, plan.columns)
		if err != nil {
			return err
		}
		pgRows.Close()

		// Recompute the explained aggregates over the identical row set
		// (count(*) rides along as the honest total behind the capped
		// rows slice). Values are read positionally, so a caller alias
		// cannot shadow the total.
		aggSQL, aggArgs, err := bindReportTokens(ctx, frame, fmt.Sprintf(
			"SELECT count(*), %s FROM %s WHERE %s",
			strings.Join(plan.aggSelects, ", "), spec.fromClause(), whereSQL), args)
		if err != nil {
			return err
		}
		values := make([]any, len(plan.aggColumns)+1)
		ptrs := make([]any, len(values))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := tx.QueryRow(ctx, aggSQL, aggArgs...).Scan(ptrs...); err != nil {
			return fmt.Errorf("derivation %s aggregates: %w", report, err)
		}
		total, ok := values[0].(int64)
		if !ok {
			return fmt.Errorf("derivation %s: count(*) returned %T, not int64", report, values[0])
		}
		out.TotalRows = int(total)
		for i, name := range plan.aggColumns {
			out.Aggregates[name] = wireValue(values[i+1])
		}
		return nil
	})
}

// maskedDerivationSelects is plan.selects with each REFERENCE column wrapped so
// that a row pointing at a record this caller cannot open returns NULL there
// instead of the id.
//
// Masking rather than excluding, and the difference is the whole of it. The
// drill-through returns every dimension the spec declares — that is deliberate,
// so a reader can see why each record is in this group — which means it hands
// back the partner id even when the number above it was a stage total. Dropping
// those rows to avoid naming the partner would make the explanation smaller
// than the number it explains. Blanking the column keeps both true: the row is
// one this caller may read, and the reference is one they may not.
//
// It mirrors what an ordinary read of the same record already does
// (deals/fieldmask.go blanks partner_org_id and keeps the deal), so a reader
// who follows the link sees the same rows with the same holes.
//
// The aggregate recompute does NOT take this treatment and must not: it counts
// rows, names no reference, and wrapping a column it never returns would only
// make the two halves read different SQL for one row set.
func maskedDerivationSelects(
	ctx context.Context, spec reportSpec, plan derivationPlan, arg func(any) int,
) ([]string, error) {
	if len(spec.referenceScopes) == 0 {
		return plan.selects, nil
	}
	// Only the references this plan actually RETURNS. A spec may scope a column
	// it exposes as a filter alone (deals-by-stage scopes organization_id for
	// that reason), and no select mentions it — while ScopeClauseFor registers a
	// bind parameter for every column it is asked about. A parameter the
	// finished statement never names is a Postgres error, not a no-op.
	returned := make(map[string]bool, len(plan.columns))
	for _, name := range plan.columns {
		if expr, ok := spec.dimensions[name]; ok {
			returned[expr] = true
		}
		if expr, ok := spec.measures[name]; ok {
			returned[expr] = true
		}
	}
	masks := make(map[string]string, len(spec.referenceScopes))
	// Sorted, so one request renders one statement. Each mask binds its own
	// parameter through arg, and map order would hand the same two references
	// different positions from run to run — an irreproducible statement in a
	// log, and no plan cache. referenceScopeClauses sorts for the same reason.
	for _, column := range slices.Sorted(maps.Keys(spec.referenceScopes)) {
		if !returned[column] {
			continue
		}
		table := spec.referenceScopes[column]
		scope, err := auth.ScopeClauseFor(ctx, table, "ref", arg)
		if err != nil {
			return nil, err
		}
		if scope == "" {
			// An unbounded reader of that table: nothing to blank.
			continue
		}
		masks[column] = fmt.Sprintf(
			"(CASE WHEN EXISTS (SELECT 1 FROM %s ref WHERE ref.id = %s AND %s) THEN %s END)",
			table, column, scope, column)
	}
	if len(masks) == 0 {
		return plan.selects, nil
	}
	// plan.columns and plan.selects are built in one pass and stay aligned, so
	// the column name at index i names the expression selected at index i.
	//
	// A column is a dimension or a measure (derivation.go appends both), and
	// both are looked up: a spec that exposed a reference as a measure would
	// otherwise return the id unmasked, which is a disclosure and not an error.
	// TestNoMeasureCarriesAReference refuses that spec at the gate, and this
	// stays honest either way rather than assuming the gate.
	out := make([]string, len(plan.selects))
	copy(out, plan.selects)
	for i, name := range plan.columns {
		expr, named := spec.dimensions[name]
		if !named {
			expr, named = spec.measures[name]
		}
		if !named {
			continue
		}
		if mask, isReference := masks[expr]; isReference {
			out[i] = mask + " AS " + name
		}
	}
	return out, nil
}

// derivationWhere renders the drill-through's WHERE side: the report's
// base predicate, the validated equality predicates ("" = SQL NULL), and
// the caller's row-scope clause (the activity link-walk when the report
// rides on activities). The identical clause backs both the rows query
// and the aggregate recompute, so the explanation can never out-see the
// number it explains.
func derivationWhere(
	ctx context.Context, tx pgx.Tx, spec reportSpec, plan derivationPlan,
	requested RequestedScope, arg func(any) int,
) ([]string, error) {
	where := []string{spec.baseWhere}
	for _, p := range plan.preds {
		switch {
		case p.threshold != nil:
			n, err := thresholdValue(p.field, p.value)
			if err != nil {
				return nil, err
			}
			where = append(where, p.threshold.clause(arg(n)))
		case p.isNull:
			where = append(where, p.expr+" IS NULL")
		default:
			where = append(where, fmt.Sprintf("%s = $%d", p.expr, arg(p.value)))
		}
	}
	var scope string
	var err error
	if spec.activityWalk {
		scope, err = auth.ActivityContentClause(ctx, "t", arg)
	} else {
		scope, err = auth.ScopeClauseFor(ctx, string(spec.entity), "t", arg)
	}
	if err != nil {
		return nil, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	// And the same POPULATION the aggregate was taken over. A drill-through
	// narrowed differently from its own headline opens records the number
	// never counted, which is the one thing an explanation must not do.
	if spec.population == measureCallersOwn {
		population, err := reportPopulationClause(ctx, tx, requested, arg)
		if err != nil {
			return nil, err
		}
		if population != "" {
			where = append(where, population)
		}
	}
	// The drill-through puts every dimension on its rows, so a reference this
	// plan RETURNS takes its row scope here — the explanation must not
	// out-see the number it explains. A reference the plan never mentions is
	// left alone, on the same terms as the headline (reportwhere.go).
	refs, err := referenceScopeClauses(ctx, spec, namedByDerivation(spec, plan), arg)
	if err != nil {
		return nil, err
	}
	return append(where, refs...), nil
}

// scanDerivationRows materializes the drill-through rows, mapping each
// column to its wire value positionally.
func scanDerivationRows(pgRows pgx.Rows, columns []string) ([]map[string]any, error) {
	var out []map[string]any
	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = wireValue(values[i])
		}
		out = append(out, row)
	}
	if err := pgRows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
