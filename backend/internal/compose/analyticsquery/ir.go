// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// The typed form a question arrives in.
//
// A model writes THIS, not SQL. Every field is either a name looked up in the
// derived schema or a value that becomes a bind parameter, so there is no
// string a caller controls that reaches the statement. That is the property
// which makes the surface safe to hand an agent, and it is a property of the
// TYPE rather than of the compiler being careful.

import (
	"fmt"
	"sort"
	"strings"
)

// Query is one question.
type Query struct {
	// Entity is the population, by name.
	Entity string
	// GroupBy are the dimensions, by name. Empty is a single-row answer over
	// the whole population, which is a real question and not a missing one.
	GroupBy []string
	// Measures are what to compute. At least one: a query with none asks for
	// group keys with nothing beside them, which is a list and not an
	// analytic question — and a list of keys is a read, gated differently.
	Measures []Measure
	// Filters narrow the population. Each names a field and carries a VALUE,
	// which becomes a bind parameter.
	Filters []Filter
	// Limit bounds the answer. Zero takes the default.
	Limit int
}

// Measure is one number to compute.
type Measure struct {
	// Fn is the aggregate.
	Fn AggFn
	// Field is what it aggregates, by name. Empty exactly for CountAll, which
	// counts rows rather than values — count(*) and count(column) differ on
	// nulls, and conflating them reports a column's coverage as its
	// population.
	Field string
	// As is the caller's name for the result. It never reaches SQL: results
	// map by POSITION, so an alias cannot carry an injection.
	As string
}

// AggFn is an aggregate this compiler renders.
//
// A CLOSED set, and small on purpose. Every entry renders a fixed SQL fragment
// with the field expression substituted from the schema; adding one is a code
// change with a test, which is what keeps the rendering surface auditable.
type AggFn string

const (
	// CountAll counts rows. The only measure that names no field.
	CountAll AggFn = "count"
	// CountDistinct counts distinct non-null values.
	CountDistinct AggFn = "count_distinct"
	// Sum adds the values.
	Sum AggFn = "sum"
	// Avg is the mean. Over a small group it approaches a single row's value,
	// which is why the floor refuses it below the threshold.
	Avg AggFn = "avg"
	// Min is the smallest value.
	Min AggFn = "min"
	// Max is the largest.
	Max AggFn = "max"
)

// aggregatesOverValues are the aggregates that need a field to aggregate.
// CountAll is the one that does not, which is why it is absent here.
var aggregatesOverValues = map[AggFn]bool{
	CountDistinct: true, Sum: true, Avg: true, Min: true, Max: true,
}

// numericAggregates are the aggregates that need a NUMBER.
//
// count_distinct, min and max are absent deliberately: distinct values, the
// earliest date and the latest stage all mean something over a non-numeric
// column, and refusing them would be this compiler inventing a restriction
// the database does not have.
var numericAggregates = map[AggFn]bool{Sum: true, Avg: true}

// Filter is one narrowing.
type Filter struct {
	// Field is the column, by name.
	Field string
	// Op is the comparison.
	Op FilterOp
	// Value becomes a bind parameter. Never rendered into the statement.
	Value any
}

// FilterOp is a comparison this compiler renders. Closed, for the same reason
// AggFn is.
type FilterOp string

// The comparisons. Each renders a fixed fragment in filterSQL below; the two
// null tests are the pair that take no value, which validateFilters holds.
const (
	// OpEq matches the value exactly.
	OpEq FilterOp = "eq"
	// OpNe matches everything else — and NOT the nulls, which SQL comparison
	// leaves out. A caller who means "including the ones with nothing there"
	// asks for is_null as well.
	OpNe FilterOp = "ne"
	// OpLt is strictly less than.
	OpLt FilterOp = "lt"
	// OpLte is less than or equal.
	OpLte FilterOp = "lte"
	// OpGt is strictly greater than.
	OpGt FilterOp = "gt"
	// OpGte is greater than or equal.
	OpGte FilterOp = "gte"
	// OpIsNull matches rows with nothing in the column.
	OpIsNull FilterOp = "is_null"
	// OpIsNotNull matches rows that have a value.
	OpIsNotNull FilterOp = "is_not_null"
)

// filterSQL renders each operator. A map rather than a switch so the set of
// renderable operators is one list a reader can check against FilterOp above.
var filterSQL = map[FilterOp]string{
	OpEq: "= %s", OpNe: "<> %s",
	OpLt: "< %s", OpLte: "<= %s", OpGt: "> %s", OpGte: ">= %s",
	OpIsNull: "IS NULL", OpIsNotNull: "IS NOT NULL",
}

// valuelessOps take no value. Given one, the query is refused rather than
// having it silently dropped: `is_null` with a value is somebody expecting a
// comparison, and answering the other question is worse than refusing.
var valuelessOps = map[FilterOp]bool{OpIsNull: true, OpIsNotNull: true}

// Validate checks the query against a schema, answering the FIRST problem.
//
// One at a time rather than all of them, because each refusal carries a
// clarification the caller acts on, and a list of five invites fixing them all
// at once against a schema they have not re-read.
func (q Query) Validate(schema Schema) error {
	entity, ok := schema.Entities[q.Entity]
	if !ok {
		return &RefusalError{
			Kind:    RefusalUnsupported,
			Message: fmt.Sprintf("no population named %q", q.Entity),
			Suggest: fmt.Sprintf("ask about one of: %s",
				strings.Join(schema.EntityNames(), ", ")),
		}
	}
	if len(q.Measures) == 0 {
		return &RefusalError{
			Kind:    RefusalInvalid,
			Message: "a question with no measure asks for group keys and nothing beside them",
			Suggest: "add a measure, or read the records directly",
		}
	}
	if err := validateNames(entity, q.GroupBy, KindDimension); err != nil {
		return err
	}
	if err := validateMeasures(entity, q.Measures); err != nil {
		return err
	}
	if err := validateAliases(q); err != nil {
		return err
	}
	return validateFilters(entity, q.Filters)
}

// validateNames checks a list of field names against one kind.
func validateNames(entity Entity, names []string, kind FieldKind) error {
	for _, name := range names {
		field, ok := entity.Lookup(name)
		if !ok {
			return unknownField(entity, name, kind)
		}
		if field.Kind != kind {
			return &RefusalError{
				Kind:    RefusalInvalid,
				Message: fmt.Sprintf("%q is a %s, not a %s", name, field.Kind, kind),
				Suggest: fmt.Sprintf("group by one of: %s",
					strings.Join(entity.FieldNames(kind), ", ")),
			}
		}
	}
	return nil
}

func validateMeasures(entity Entity, measures []Measure) error {
	for _, m := range measures {
		if _, known := knownAggregates[m.Fn]; !known {
			return &RefusalError{
				Kind:    RefusalUnsupported,
				Message: fmt.Sprintf("no aggregate named %q", m.Fn),
				Suggest: "use one of: " + strings.Join(aggregateNames(), ", "),
			}
		}
		if !aggregatesOverValues[m.Fn] {
			// CountAll. A field alongside it is a caller expecting
			// count(column), which differs on nulls — answering the other
			// question would report a column's coverage as its population.
			if m.Field != "" {
				return &RefusalError{
					Kind:    RefusalInvalid,
					Message: "count takes no field; it counts rows",
					Suggest: fmt.Sprintf("use count_distinct on %q to count values", m.Field),
				}
			}
			continue
		}
		field, ok := entity.Lookup(m.Field)
		if !ok {
			return unknownField(entity, m.Field, KindMeasure)
		}
		if numericAggregates[m.Fn] && field.Kind != KindMeasure {
			return &RefusalError{
				Kind:    RefusalInvalid,
				Message: fmt.Sprintf("%s over %q, which is a %s", m.Fn, m.Field, field.Kind),
				Suggest: fmt.Sprintf("%s one of: %s", m.Fn,
					strings.Join(entity.FieldNames(KindMeasure), ", ")),
			}
		}
	}
	return nil
}

// ReservedColumns are the names the engine itself writes into an answer.
//
// A caller aliasing a measure to one of these was, before this check, aliasing
// it over the privacy floor's own input — the floor then judged a group by that
// measure's value and served a two-record group whole. The executor no longer
// reads by name, so this is the second lock; both, because a reader of either
// half should not have to check the other to know the collision is handled.
var ReservedColumns = []string{"_rows", "_withheld"}

// validateAliases refuses a result column that would shadow another.
//
// Duplicates and reserved names alike. Two columns with one name is not a
// question with an answer: whichever the caller meant, one of the two numbers
// they asked for is missing and nothing says which.
func validateAliases(q Query) error {
	seen := map[string]bool{}
	for _, name := range q.GroupBy {
		seen[name] = true
	}
	for _, m := range q.Measures {
		name := measureAlias(m)
		for _, reserved := range ReservedColumns {
			if name == reserved {
				return &RefusalError{
					Kind:    RefusalInvalid,
					Message: fmt.Sprintf("%q is the engine's own column", name),
					Suggest: "name the result something else",
				}
			}
		}
		if seen[name] {
			return &RefusalError{
				Kind:    RefusalInvalid,
				Message: fmt.Sprintf("two result columns would both be called %q", name),
				Suggest: "give each measure its own name",
			}
		}
		seen[name] = true
	}
	return nil
}

// measureAlias is what a measure's column will be called — the same answer
// measureName gives in the compiler, asked here so the check judges the name
// that actually ships.
func measureAlias(m Measure) string {
	if m.As != "" {
		return m.As
	}
	if m.Field == "" {
		return string(m.Fn)
	}
	return string(m.Fn) + "_" + m.Field
}

func validateFilters(entity Entity, filters []Filter) error {
	for _, f := range filters {
		if _, ok := filterSQL[f.Op]; !ok {
			return &RefusalError{
				Kind:    RefusalUnsupported,
				Message: fmt.Sprintf("no comparison named %q", f.Op),
				Suggest: "use one of: " + strings.Join(filterOpNames(), ", "),
			}
		}
		if _, ok := entity.Lookup(f.Field); !ok {
			return unknownField(entity, f.Field, KindDimension)
		}
		if valuelessOps[f.Op] != (f.Value == nil) {
			return &RefusalError{
				Kind: RefusalInvalid,
				Message: fmt.Sprintf(
					"%s on %q was given the wrong shape: %s takes %s",
					f.Op, f.Field, f.Op, valueShape(f.Op)),
				Suggest: "drop the value, or use a comparison that takes one",
			}
		}
	}
	return nil
}

func valueShape(op FilterOp) string {
	if valuelessOps[op] {
		return "no value"
	}
	return "a value"
}

// unknownField is the one answer for a field that does not exist AND for one
// the caller may not read.
//
// The same sentence for both, deliberately. "You may not read that" tells
// somebody a column exists, which is the disclosure the grant was for. What
// they get instead is the list of what they CAN name, which is both more
// useful and says nothing about the rest.
func unknownField(entity Entity, name string, kind FieldKind) error {
	return &RefusalError{
		Kind:    RefusalUnsupported,
		Message: fmt.Sprintf("no field named %q on %s", name, entity.Name),
		Suggest: fmt.Sprintf("available: %s", strings.Join(entity.FieldNames(kind), ", ")),
	}
}

// knownAggregates answers "is this an aggregate at all", which Validate asks
// before the narrower questions about fields and types. A name absent here is
// refused as unsupported rather than reaching the renderer.
var knownAggregates = map[AggFn]bool{
	CountAll: true, CountDistinct: true, Sum: true, Avg: true, Min: true, Max: true,
}

func aggregateNames() []string {
	out := make([]string, 0, len(knownAggregates))
	for fn := range knownAggregates {
		out = append(out, string(fn))
	}
	sort.Strings(out)
	return out
}

func filterOpNames() []string {
	out := make([]string, 0, len(filterSQL))
	for op := range filterSQL {
		out = append(out, string(op))
	}
	sort.Strings(out)
	return out
}
