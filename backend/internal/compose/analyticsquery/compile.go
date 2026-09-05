// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// Turning a validated query into a parameterised statement.
//
// The one rule this file exists to keep: NOTHING FROM THE REQUEST IS FORMATTED
// INTO THE SQL. Every identifier is an expression read out of the schema, which
// was built from the spec catalog; every value is a bind parameter. A caller
// who sends a field name containing a semicolon does not reach this code — the
// name is looked up, found absent, and refused.
//
// The statement is assembled in one pass with one argument slice, so a bind
// position and the value at it cannot drift apart. Positions are derived from
// the slice's length rather than counted by hand, which is the rule the
// rulebook states as never hand-typing a placeholder.

import (
	"fmt"
	"strings"
)

// Plan is a compiled query: the statement, its arguments, and what the columns
// mean.
type Plan struct {
	SQL  string
	Args []any
	// Columns names each selected column IN ORDER — the group keys, then the
	// measures. Results map by position, which is what lets a caller's alias
	// stay out of the SQL entirely.
	Columns []string
	// GroupCount is how many leading columns are group keys. The privacy floor
	// needs to know which columns identify a group and which are its numbers.
	GroupCount int
	// CountColumn is the position of the row count the floor is judged on.
	// Every plan carries one whether or not the caller asked for it: a floor
	// that could only be applied when somebody happened to request a count
	// would be a floor that a caller turns off by not asking.
	CountColumn int
	// ExcludedSQL counts the rows this query's FILTERS removed from the
	// population, with ExcludedArgs as its own bind list.
	//
	// It exists because the floor is otherwise defeated across two queries
	// rather than inside one. Ask for a total, then ask again with
	// `owner_id ne <someone>`; the difference is that owner's exact figure, and
	// each answer on its own covers a large enough group that the floor sees
	// nothing to withhold. Counting what the filter took out is what makes the
	// second query refusable.
	//
	// Empty when the query has no filters of its own, which removes nothing and
	// so discloses nothing by difference.
	ExcludedSQL  string
	ExcludedArgs []any
}

// defaultLimit bounds an answer nobody bounded.
//
// A generic query has no catalog entry saying how many groups are reasonable,
// so the ceiling is here. It is a LIMIT rather than a refusal because a
// hundred groups is a real answer to a real question; what it prevents is a
// grouping by a high-cardinality column returning a row per record.
const defaultLimit = 500

// maxLimit is the most a caller may ask for.
const maxLimit = 5000

// ScopeClauses yields every row-level narrowing the caller's authority puts on
// this population, as SQL over the entity's own alias, appending binds through
// arg.
//
// Injected rather than reached for: this package owns no auth. What it owns is
// the guarantee that a compiled plan CANNOT be built without asking — a nil
// here is refused rather than treated as "nothing to narrow", because the two
// look identical at a call site and only one of them is safe.
// The int is the bind POSITION, matching the shape the report engine's own
// clause builders take, so a caller can hand them this function unchanged.
type ScopeClauses func(arg func(any) int) ([]string, error)

// Compile renders a validated query.
//
// Validate is called again here rather than trusted from the caller: this is
// the function that builds SQL, and a compiler that renders whatever it is
// handed is one refactor away from rendering something nobody checked.
func Compile(q Query, schema Schema, scope ScopeClauses) (Plan, error) {
	if scope == nil {
		return Plan{}, &RefusalError{
			Kind:    RefusalInvalid,
			Message: "a query was compiled with no scope source",
			Suggest: "this is a wiring fault rather than a question anybody asked",
		}
	}
	if err := q.Validate(schema); err != nil {
		return Plan{}, err
	}
	entity := schema.Entities[q.Entity]

	var args []any
	// Positions are DERIVED from the slice's length, never counted by hand.
	// The rulebook states it as never hand-typing a placeholder, and the
	// failure it prevents is a statement whose column, placeholder and
	// argument counts disagree with nothing to notice.
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }

	selects, columns := groupSelects(entity, q.GroupBy)
	groupCount := len(columns)

	// The row count goes in every plan, at a known position, so the floor is
	// applied to a number the query always has rather than one the caller
	// chose to ask for.
	countColumn := len(columns)
	selects = append(selects, "count(*)")
	columns = append(columns, countRowsColumn)

	measureSelects, measureColumns, err := measureSelects(entity, q.Measures)
	if err != nil {
		return Plan{}, err
	}
	selects = append(selects, measureSelects...)
	columns = append(columns, measureColumns...)

	where, err := whereClauses(entity, q.Filters, bind)
	if err != nil {
		return Plan{}, err
	}
	// The caller's authority over this population: their row scope, the
	// activity content walk where the population is message bodies, the
	// excluded masked rows, and the references they may not name. Asked HERE
	// so no path renders a statement without them.
	scoped, err := scope(arg)
	if err != nil {
		return Plan{}, err
	}
	where = append(where, scoped...)

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selects, ", "), entity.From)
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if groupCount > 0 {
		sql += " GROUP BY " + groupPositions(groupCount)
	}
	// Ordering by the group keys rather than by a measure, so paging is
	// deterministic: an answer ordered by a total re-orders the moment a deal
	// moves, and two pages of one query then overlap or skip.
	if groupCount > 0 {
		sql += " ORDER BY " + groupPositions(groupCount)
	}
	sql += fmt.Sprintf(" LIMIT %s", bind(boundedLimit(q.Limit)))

	plan := Plan{
		SQL: sql, Args: args, Columns: columns,
		GroupCount: groupCount, CountColumn: countColumn,
	}
	plan.ExcludedSQL, plan.ExcludedArgs, err = excludedCount(entity, q, scope)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// excludedCount renders the statement that counts what the filters removed.
//
// The SAME population and the SAME authority narrowings as the answer itself,
// with the caller's filters NEGATED. That is the set a second, unfiltered query
// would let somebody subtract their way to, so it is the set the floor has to
// judge before this answer is served.
//
// A query with no filters removes nothing: no statement, and nothing to check.
func excludedCount(entity Entity, q Query, scope ScopeClauses) (string, []any, error) {
	if len(q.Filters) == 0 {
		return "", nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }

	filters, err := whereClauses(entity, q.Filters, bind)
	if err != nil {
		return "", nil, err
	}
	// whereClauses puts the population's own definition first, and that is NOT
	// part of what the caller excluded — it defines the population rather than
	// narrowing within it. Negating it would count every archived deal in the
	// database as "withheld by this filter".
	if entity.BaseWhere != "" {
		filters = filters[1:]
	}
	scoped, err := scope(arg)
	if err != nil {
		return "", nil, err
	}

	where := []string{}
	if entity.BaseWhere != "" {
		where = append(where, entity.BaseWhere)
	}
	where = append(where, scoped...)
	where = append(where, "NOT ("+strings.Join(filters, " AND ")+")")
	return fmt.Sprintf("SELECT count(*) FROM %s WHERE %s",
		entity.From, strings.Join(where, " AND ")), args, nil
}

// countRowsColumn is the name of the count every plan carries. Prefixed so it
// cannot collide with a caller's alias — which would be a column the caller
// reads as their own measure and is in fact the floor's.
const countRowsColumn = "_rows"

// groupSelects renders the group keys.
func groupSelects(entity Entity, names []string) ([]string, []string) {
	selects := make([]string, 0, len(names))
	columns := make([]string, 0, len(names))
	for _, name := range names {
		// The expression comes from the schema, which came from the spec.
		// `name` was already looked up by Validate; it is used here only as
		// the map key and as the caller-facing column name, never as SQL.
		selects = append(selects, entity.Fields[name].Expr)
		columns = append(columns, name)
	}
	return selects, columns
}

// measureSelects renders the aggregates.
func measureSelects(entity Entity, measures []Measure) ([]string, []string, error) {
	selects := make([]string, 0, len(measures))
	columns := make([]string, 0, len(measures))
	for _, m := range measures {
		expr, err := aggregateSQL(entity, m)
		if err != nil {
			return nil, nil, err
		}
		selects = append(selects, expr)
		columns = append(columns, measureName(m))
	}
	return selects, columns, nil
}

// aggregateSQL renders one aggregate over a schema expression.
func aggregateSQL(entity Entity, m Measure) (string, error) {
	if m.Fn == CountAll {
		return "count(*)", nil
	}
	field, ok := entity.Lookup(m.Field)
	if !ok {
		// Validate already refused this. Kept because this function renders
		// SQL, and a renderer that assumes its input was checked is one
		// caller away from rendering an empty expression into a statement.
		return "", unknownField(entity, m.Field, KindMeasure)
	}
	if m.Fn == CountDistinct {
		return fmt.Sprintf("count(DISTINCT %s)", field.Expr), nil
	}
	if m.Fn == Median || m.Fn == P75 {
		return PercentileExpr(field.Expr, m.Fn), nil
	}
	return fmt.Sprintf("%s(%s)", m.Fn, field.Expr), nil
}

// PercentileExpr renders a percentile over a TRUSTED schema expression. Both
// engines call it — the report engine's median/p75 case and the typed
// compiler below — which is what keeps the screen and the typed query giving
// one answer to "what is a median" instead of two.
//
// NULL below the sample floor rather than a number. A median over three deals
// is not a median: it is one deal's value wearing a statistic's name, and a
// manager comparing "typical stage age" across teams would read the smallest
// team's outlier as its norm. Postgres will happily compute it, which is
// exactly why the refusal has to be rendered here. NULL rather than an error,
// because the row is still a real answer — the count beside the blank says
// how many deals there were, and failing the whole query would take that too.
func PercentileExpr(expr string, fn AggFn) string {
	fraction := "0.5"
	if fn == P75 {
		fraction = "0.75"
	}
	return fmt.Sprintf(
		"(CASE WHEN count(%s) >= %d THEN percentile_cont(%s) WITHIN GROUP (ORDER BY %s) END)",
		expr, PercentileSampleFloor, fraction, expr)
}

// PercentileSampleFloor is how many values a percentile needs before it means
// anything. Five is a judgement, not a derivation: the point of a percentile
// here is comparing groups of differing sizes, and below a handful of values
// the comparison reads noise as signal.
const PercentileSampleFloor = 5

// measureName is what the caller calls the column, and it is measureAlias —
// one spelling, because validateAliases refuses a collision and this renders
// it, and two answers to "what is this column called" would let a name pass
// the check under one spelling and ship under another.
func measureName(m Measure) string { return measureAlias(m) }

// whereClauses renders the filters, values bound.
func whereClauses(entity Entity, filters []Filter, arg func(any) string) ([]string, error) {
	var out []string
	if entity.BaseWhere != "" {
		// The population's own definition — what the report means by "open
		// deals". A generic query over an entity answers the same population
		// the prebuilt report does, rather than a wider one that happens to
		// share a table.
		out = append(out, entity.BaseWhere)
	}
	for _, f := range filters {
		field, ok := entity.Lookup(f.Field)
		if !ok {
			return nil, unknownField(entity, f.Field, KindDimension)
		}
		form := filterSQL[f.Op]
		if valuelessOps[f.Op] {
			out = append(out, fmt.Sprintf("%s %s", field.Expr, form))
			continue
		}
		out = append(out, fmt.Sprintf("%s %s", field.Expr, fmt.Sprintf(form, arg(f.Value))))
	}
	return out, nil
}

// groupPositions renders GROUP BY and ORDER BY as ordinals.
//
// Positions rather than repeating the expressions: an expression written twice
// is one that can come to be written two ways, and Postgres would then group by
// something other than what it selected.
func groupPositions(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprint(i + 1)
	}
	return strings.Join(parts, ", ")
}

func boundedLimit(want int) int {
	if want <= 0 {
		return defaultLimit
	}
	if want > maxLimit {
		return maxLimit
	}
	return want
}
