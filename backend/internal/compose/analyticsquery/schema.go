// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// The vocabulary a question may be asked in.
//
// Derived rather than declared: the entities, dimensions and measures come from
// the report specs the engine already runs, so a dimension that exists here is
// one the prebuilt reports can group by and one the row-scope machinery already
// knows how to gate. A second hand-written catalog would be a second answer to
// "what can be asked", and the two would drift the first time somebody added a
// column to one of them.
//
// The schema is derived PER CALLER. A field the caller may not read is not in
// their schema at all, so naming it is "no such field" rather than "you may not
// read that" — the second sentence tells somebody what exists.

import "sort"

// Field is one thing that can be grouped by or measured.
type Field struct {
	Name string
	// Expr is the SQL the compiler renders. It comes from the spec catalog and
	// never from a request; nothing a caller sends reaches this string.
	Expr string
	// Kind decides which measures apply. A sum over a text column is a
	// question with no answer, and refusing it at plan time beats a Postgres
	// error the caller cannot act on.
	Kind FieldKind
}

// FieldKind is what a field holds.
type FieldKind string

const (
	// KindDimension is something to group by: a stage, an owner, a currency.
	KindDimension FieldKind = "dimension"
	// KindMeasure is something to aggregate: an amount, an age, a count.
	KindMeasure FieldKind = "measure"
)

// Entity is one population a question can be asked about.
type Entity struct {
	Name string
	// From is the whole FROM side — the base table, its alias, and the spec's
	// fixed lookup joins — taken from the spec rather than rebuilt. A spec
	// never joins a to-many side, so the row grain stays the base table's and
	// an aggregate here cannot double-count; that property is inherited rather
	// than re-established.
	From string
	// BaseWhere narrows the population to what the report means by it: open
	// deals, live activities. Carried so a generic query over an entity
	// answers the same population the prebuilt report does.
	BaseWhere string
	Fields    map[string]Field
}

// Schema is the whole vocabulary, for one caller.
type Schema struct {
	Entities map[string]Entity
	// Version changes when the derivation changes. A compiled plan names the
	// version it was planned against and is refused after it moves: a plan
	// naming a field that has since been renamed would otherwise render SQL
	// against a column that no longer exists, and the caller would read a
	// database error instead of "re-plan this".
	Version string
}

// EntityNames lists the populations, in a stable order.
//
// Sorted rather than map order, because this reaches a model as a list and an
// order that changes per call makes two identical questions look different —
// which breaks the result digest that says two runs computed the same thing.
func (s Schema) EntityNames() []string {
	out := make([]string, 0, len(s.Entities))
	for name := range s.Entities {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// FieldNames lists one entity's fields of a kind, in a stable order.
func (e Entity) FieldNames(kind FieldKind) []string {
	out := make([]string, 0, len(e.Fields))
	for name, field := range e.Fields {
		if field.Kind == kind {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Lookup answers a field by name, and whether the schema has it.
//
// The bool is the whole point: a caller naming a field that is absent — because
// it does not exist, or because their grants removed it — gets the same answer
// either way, so a refusal never says which.
func (e Entity) Lookup(name string) (Field, bool) {
	field, ok := e.Fields[name]
	return field, ok
}
