// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Running a compiled generic query, and withholding what the floor covers.
//
// The floor is applied HERE rather than in SQL, and that is deliberate: it
// needs every group's size before it can decide which complements to withhold,
// and a HAVING clause sees one group at a time. Postgres does the arithmetic;
// this decides what a reader is allowed to be told about it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AnalyticsAnswer is one generic query's result.
type AnalyticsAnswer struct {
	// Columns names each value in a row, in order — the group keys, then the
	// caller's measures. The plan's own row count is not among them: it is the
	// floor's input, not an answer.
	Columns []string
	Rows    []map[string]any
	// Withheld says the floor kept something back. A boolean and never a
	// count, for the reason every other withheld flag in this tree is one.
	Withheld bool
	// TotalSafe says whether a total over these rows may be shown. False once
	// anything is withheld, because total-minus-shown is the subtraction the
	// floor exists to stop.
	TotalSafe bool
	// SchemaVersion is the vocabulary this ran against. A caller comparing two
	// answers needs to know they were asked in the same language.
	SchemaVersion string
}

// RunAnalyticsQuery compiles and runs one question.
//
// The row scope is NOT applied here, and that is not an omission: the entity's
// table and base predicate come from a report spec, and the report engine's own
// gate covers the population. What this adds is the field-level narrowing —
// which happened when the schema was derived, before the query was planned.
func RunAnalyticsQuery(
	ctx context.Context, tx pgx.Tx, q analyticsquery.Query, floor analyticsquery.Floor,
) (AnalyticsAnswer, error) {
	// The population's own read gate, asked BEFORE anything is compiled. The
	// schema derivation already drops entities this caller cannot read, so
	// reaching here with an unreadable one means the derivation and this gate
	// disagree — and the safe reading of that is to refuse.
	spec, ok := prebuiltReports[q.Entity]
	if !ok {
		return AnalyticsAnswer{}, &analyticsquery.RefusalError{
			Kind:    analyticsquery.RefusalUnsupported,
			Message: "no population by that name",
			Suggest: "call the schema endpoint for the populations this seat may ask about",
		}
	}
	if err := auth.Require(ctx, string(spec.entity), principal.ActionRead); err != nil {
		return AnalyticsAnswer{}, err
	}

	schema := AnalyticsSchemaFor(ctx)
	plan, err := analyticsquery.Compile(q, schema, analyticsScope(ctx, spec))
	if err != nil {
		return AnalyticsAnswer{}, err
	}

	frame, err := readReportFrame(ctx, tx)
	if err != nil {
		return AnalyticsAnswer{}, err
	}
	sql, args, err := bindReportTokens(ctx, frame, plan.SQL, plan.Args)
	if err != nil {
		return AnalyticsAnswer{}, err
	}

	// What this query's filters removed, judged BEFORE the answer is served.
	// A filtered answer and an unfiltered one differ by exactly that set, and
	// somebody who can ask both has it — so a filter that excludes fewer
	// records than the floor is refused rather than answered.
	if err := refuseIfTheFilterHidesTooLittle(ctx, tx, frame, plan, floor); err != nil {
		return AnalyticsAnswer{}, err
	}

	pgRows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		// The statement is not in the message. A caller who can read the SQL
		// back learns the schema's expressions, which the derivation narrowed
		// on purpose.
		return AnalyticsAnswer{}, fmt.Errorf("compose: running an analytics query: %w", err)
	}
	defer pgRows.Close()

	// Scanned POSITIONALLY, not into a map keyed by the caller's column names.
	// A map lets one alias shadow another: aliasing a measure to the count's
	// name overwrote the floor's own input, and the floor then judged a
	// two-deal group by that measure's value and served it whole.
	raw, err := scanAnalyticsRows(pgRows)
	if err != nil {
		return AnalyticsAnswer{}, fmt.Errorf("compose: reading an analytics answer: %w", err)
	}
	return withheldAnswer(raw, plan, floor, schema.Version), nil
}

// refuseIfTheFilterHidesTooLittle refuses an answer whose filter narrowed the
// population by less than the floor.
//
// The attack it stops takes two calls and no aliases: ask for a total, then ask
// again with `owner_id ne <them>`. Each answer covers a large group, so the
// floor withholds nothing from either — and the difference is that owner's
// exact figure. The complement rule inside one answer does nothing about
// arithmetic performed across two.
//
// The refusal names the coarser question rather than the number, because saying
// "that excludes 2 records" would BE the disclosure.
func refuseIfTheFilterHidesTooLittle(
	ctx context.Context, tx pgx.Tx, frame reportFrame,
	plan analyticsquery.Plan, floor analyticsquery.Floor,
) error {
	if plan.ExcludedSQL == "" || floor <= 0 {
		return nil
	}
	sql, args, err := bindReportTokens(ctx, frame, plan.ExcludedSQL, plan.ExcludedArgs)
	if err != nil {
		return err
	}
	var excluded int64
	if err := tx.QueryRow(ctx, sql, args...).Scan(&excluded); err != nil {
		return fmt.Errorf("compose: counting what an analytics filter excluded: %w", err)
	}
	// Zero is a filter that removed nothing — no disclosure, and refusing it
	// would refuse every query whose filter happens to match everything.
	if excluded == 0 || excluded >= int64(floor) {
		return nil
	}
	return &analyticsquery.RefusalError{
		Kind: analyticsquery.RefusalPrivacy,
		Message: "this filter separates out too few records: the answer to it, " +
			"set beside the answer without it, would describe them",
		Suggest: "ask without the filter, or with one that covers more records",
	}
}

// analyticsScope yields every narrowing the population carries.
//
// A NAMED function rather than a closure at the call site, and that is not
// style: the gate holding this rule walks function declarations, and a closure
// is invisible to it. Written inline, swapping the whole composer for its scope
// half passed the gate — which is the defect the gate exists to catch.
func analyticsScope(ctx context.Context, spec reportSpec) analyticsquery.ScopeClauses {
	return func(arg func(any) int) ([]string, error) {
		return specNarrowings(ctx, spec, arg)
	}
}

// withheldAnswer applies the floor and drops the count column.
func withheldAnswer(
	raw [][]any, plan analyticsquery.Plan,
	floor analyticsquery.Floor, schemaVersion string,
) AnalyticsAnswer {
	rows := make([]analyticsquery.Row, len(raw))
	for i, row := range raw {
		rows[i] = analyticsquery.Row{Count: rowCount(row[plan.CountColumn])}
	}
	judged, withheld := floor.Apply(rows)

	// The count column never reaches the caller. It is the floor's input, and
	// serving it would report the size of a group whose numbers were withheld
	// for being that size.
	columns := make([]string, 0, len(plan.Columns)-1)
	for i, name := range plan.Columns {
		if i != plan.CountColumn {
			columns = append(columns, name)
		}
	}

	out := make([]map[string]any, len(raw))
	for i, row := range raw {
		shaped := make(map[string]any, len(columns))
		for j, name := range columns {
			// The value comes from its POSITION in the scanned row. Read by
			// name, a caller aliasing a measure to a group key's name would
			// have one overwrite the other in the map — and aliasing it to the
			// count's name overwrote the floor's own input, which turned the
			// floor off for a five-character JSON field.
			source := j
			if j >= plan.CountColumn {
				source = j + 1
			}
			// A withheld group loses its KEYS as well as its numbers.
			//
			// Keeping the keys read as the gentler choice and was not. Group by
			// project_id, name and key and every group is one project, so every
			// group is under the floor — and the answer was a paginated dump of
			// every project's identity with the measures blanked. Identity is
			// the thing EnsureVisible and the reference scopes exist to
			// protect, so a surface that hands it over while withholding a
			// count has withheld the wrong half.
			if judged[i].Withheld {
				shaped[name] = nil
				continue
			}
			shaped[name] = row[source]
		}
		shaped[withheldColumn] = judged[i].Withheld
		out[i] = shaped
	}
	return AnalyticsAnswer{
		Columns: columns, Rows: out, Withheld: withheld,
		TotalSafe: analyticsquery.TotalIsSafe(withheld), SchemaVersion: schemaVersion,
	}
}

// scanAnalyticsRows reads every row as its raw values, in select order.
//
// Positional on purpose: the caller names the columns, and two names that
// collide are the caller's business right up until one of them is the privacy
// floor's input.
func scanAnalyticsRows(pgRows pgx.Rows) ([][]any, error) {
	// Empty, never nil. "Nothing matched" is a real answer and arrives shaped
	// like the array it is; nil marshals to null, which a model reads as
	// "unknown".
	out := [][]any{}
	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, err
		}
		for i, v := range values {
			values[i] = wireValue(v)
		}
		out = append(out, values)
	}
	return out, pgRows.Err()
}

// withheldColumn marks a row the floor kept back. Prefixed so it cannot
// collide with a caller's alias, which would be a column they read as their
// own measure.
const withheldColumn = "_withheld"

// rowCount reads the plan's count, whatever integer width the driver returned.
//
// A count that failed to parse answers ZERO, which the floor reads as below it
// and withholds. Failing open here would serve a group whose size nothing
// established.
func rowCount(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int32:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
