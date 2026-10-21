// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// Resolving one cell of an answer back to the records it was computed from.
//
// A number on a revenue screen is only worth having if somebody can open it,
// and "open it" means the ROWS — not a restatement of the formula. So an
// explanation is the same population, the same authority narrowings and the
// same filters as the answer, plus the group this cell belongs to.
//
// THE EXPLANATION MUST NOT OUT-SEE THE NUMBER. It reads the identical row set,
// which is why it is built from the same Query rather than from a description
// of it: a second assembly would drift, and the direction it drifts is a
// drill-through showing rows that were excluded from the total above it.
//
// A WITHHELD CELL EXPLAINS TO NOTHING. The floor withheld the group because it
// covers too few records; handing over those records one at a time is the same
// disclosure at a slower pace.

import (
	"fmt"
	"strings"
)

// Explain is a request to resolve one cell.
type Explain struct {
	// Query is the question the cell came from, unchanged. Carried whole
	// rather than as a handle the caller could edit: an explanation of a
	// DIFFERENT query than the one that produced the number is not an
	// explanation, and nothing downstream could tell.
	Query Query
	// Group binds the cell's group keys — one entry per GroupBy dimension, in
	// the same order. Empty for an ungrouped answer, which has one cell.
	Group []any
	// AllCells explains every cell at once: the question keeps its grouping,
	// and with it every narrowing the grouping brings, and only the one-cell
	// restriction is dropped. Group must then be empty.
	AllCells bool
}

// ExplainPlan is the compiled drill-through.
type ExplainPlan struct {
	SQL     string
	Args    []any
	Columns []string
}

// ExplainRowLimit bounds how many records a cell resolves to.
//
// A cell over ten thousand deals cannot be read row by row, and returning them
// would be a dump wearing an explanation's name. Exported so the caller can
// tell a full page from a truncated one without spelling the number twice, and
// the answer says which so a reader knows they are seeing part.
const ExplainRowLimit = 200

// CompileExplain renders the drill-through for one cell.
//
// The same scope source as the answer, because the two must read one row set.
// Passing a different one here would be the whole defect this function exists
// to avoid, so it takes the same type the compiler does.
func CompileExplain(in Explain, schema Schema, scope ScopeClauses) (ExplainPlan, error) {
	if scope == nil {
		return ExplainPlan{}, &RefusalError{
			Kind:    RefusalInvalid,
			Message: "an explanation was compiled with no scope source",
			Suggest: "this is a wiring fault rather than a question anybody asked",
		}
	}
	if err := in.Query.Validate(schema); err != nil {
		return ExplainPlan{}, err
	}
	if in.AllCells && len(in.Group) > 0 {
		return ExplainPlan{}, &RefusalError{
			Kind:    RefusalInvalid,
			Message: "an explanation of every cell names no single cell",
			Suggest: "name one cell, or ask for every cell with no group",
		}
	}
	if !in.AllCells && len(in.Group) != len(in.Query.GroupBy) {
		return ExplainPlan{}, &RefusalError{
			Kind: RefusalInvalid,
			Message: fmt.Sprintf(
				"the cell names %d group value(s) and the question grouped by %d",
				len(in.Group), len(in.Query.GroupBy)),
			Suggest: "pass the cell's own group keys, in the order the answer listed them",
		}
	}
	entity := schema.Entities[in.Query.Entity]

	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }

	where, err := whereClauses(entity, in.Query.Filters, bind)
	if err != nil {
		return ExplainPlan{}, err
	}
	scoped, err := scope(arg)
	if err != nil {
		return ExplainPlan{}, err
	}
	where = append(where, scoped...)

	// The cell's own group. A NULL key is bound as IS NULL rather than `= NULL`
	// — which is never true — so a cell whose group is "unset" resolves to the
	// rows that have nothing there rather than to none at all.
	for i, name := range in.Query.GroupBy {
		if in.AllCells {
			break
		}
		expr := entity.Fields[name].Expr
		if in.Group[i] == nil {
			where = append(where, expr+" IS NULL")
			continue
		}
		where = append(where, expr+" = "+bind(in.Group[i]))
	}

	// The identifying column plus every dimension the question named, so a
	// reader can see WHY each record is in this group. The measures are not
	// re-listed: the cell above already carries them, and computing them again
	// here is a second answer to one question.
	selects := []string{"t.id"}
	columns := []string{"id"}
	for _, name := range in.Query.GroupBy {
		selects = append(selects, entity.Fields[name].Expr)
		columns = append(columns, name)
	}
	for _, m := range in.Query.Measures {
		if m.Field == "" {
			continue
		}
		selects = append(selects, entity.Fields[m.Field].Expr)
		columns = append(columns, m.Field)
	}

	// Ordered by id and bounded. Deterministic because a reader who pages
	// through an explanation twice must see the same records in the same
	// order, and an order over a measure re-sorts the moment a record changes.
	sql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY t.id LIMIT %s",
		strings.Join(selects, ", "), entity.From,
		whereSQL(where), bind(ExplainRowLimit))
	return ExplainPlan{SQL: sql, Args: args, Columns: columns}, nil
}

// SourceFilter narrows a source listing. Only restricts it to those record
// ids, so a kept answer's records can be judged again under a later reader.
// Groups restricts it to the records of those groups, one key per GroupBy
// field each. Nil leaves every group in. An empty list lists nothing.
type SourceFilter struct {
	Only   []string
	Groups *[][]any
}

// CompileSources renders every record behind an answer, each with the group
// keys that place it in a row. It reads the answer's own row set, as an
// explanation does. The caller passes one past its bound as limit, to tell a
// full list from a cut one. The order by id makes a cut list the same twice.
func CompileSources(q Query, schema Schema, scope ScopeClauses, filter SourceFilter, limit int) (ExplainPlan, error) {
	if scope == nil {
		return ExplainPlan{}, &RefusalError{
			Kind:    RefusalInvalid,
			Message: "a source listing was compiled with no scope source",
			Suggest: "this is a wiring fault rather than a question anybody asked",
		}
	}
	if err := q.Validate(schema); err != nil {
		return ExplainPlan{}, err
	}
	entity := schema.Entities[q.Entity]

	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }

	where, err := whereClauses(entity, q.Filters, bind)
	if err != nil {
		return ExplainPlan{}, err
	}
	scoped, err := scope(arg)
	if err != nil {
		return ExplainPlan{}, err
	}
	where = append(where, scoped...)
	if filter.Only != nil {
		where = append(where, "t.id = ANY("+bind(filter.Only)+"::uuid[])")
	}
	if filter.Groups != nil {
		where = append(where, groupsClause(entity, q.GroupBy, *filter.Groups, bind))
	}

	selects := []string{"t.id"}
	columns := []string{"id"}
	for _, name := range q.GroupBy {
		selects = append(selects, entity.Fields[name].Expr)
		columns = append(columns, name)
	}
	sql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY t.id LIMIT %s",
		strings.Join(selects, ", "), entity.From, whereSQL(where), bind(limit))
	return ExplainPlan{SQL: sql, Args: args, Columns: columns}, nil
}

// groupsClause matches a record to any of the groups. IS NOT DISTINCT FROM,
// because a group whose key is unset must match its records, which = never does.
func groupsClause(entity Entity, groupBy []string, groups [][]any, bind func(any) string) string {
	if len(groups) == 0 {
		return "FALSE"
	}
	rows := make([]string, len(groups))
	for i, keys := range groups {
		parts := make([]string, len(groupBy))
		for j, name := range groupBy {
			parts[j] = entity.Fields[name].Expr + " IS NOT DISTINCT FROM " + bind(keys[j])
		}
		rows[i] = "(" + strings.Join(parts, " AND ") + ")"
	}
	return "(" + strings.Join(rows, " OR ") + ")"
}
