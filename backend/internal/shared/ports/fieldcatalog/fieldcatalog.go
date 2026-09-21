// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package fieldcatalog is the cross-module seam a record store rides to consume
// custom-field columns without importing modules/customfields. That module owns
// the custom_field table and implements Reader; compose injects it, and a nil
// Reader is the zero-cost pass-through for an unwired seam.
//
// Column is deliberately thin: enough for a store's SQL mechanics to build a
// fragment and convert a wire value to its bind shape. Admin-facing catalog
// metadata stays inside modules/customfields.
package fieldcatalog

import "context"

// The six closed field types (custom-fields.md), spelled the way
// modules/customfields' own type constants and the custom_field.type
// CHECK constraint spell them. Shared may not import modules (it would
// invert the shared → platform → modules DAG), so this is the one other
// place these six literals are allowed to live — modules/customfields
// and platform/database/storekit both consume this set rather than
// hand-rolling their own copies.
const (
	TypeText        = "text"
	TypeNumber      = "number"
	TypeDate        = "date"
	TypeCurrency    = "currency"
	TypePicklist    = "picklist"
	TypeMultiselect = "multiselect"
	TypeBoolean     = "boolean"
)

// Types answers the closed set above, so a consumer handling EVERY field type
// derives that obligation instead of restating it: a gate over a hand-copied
// list passes unchanged the day a seventh type lands, which is the one moment it
// exists to fail. Missing the segment vocabulary costs a column its filter;
// missing storekit's conversion matrix costs the VALUE on both write and read.
//
// A fresh slice per call, so no consumer can reorder it for every other.
func Types() []string {
	return []string{TypeText, TypeNumber, TypeDate, TypeCurrency, TypePicklist, TypeMultiselect, TypeBoolean}
}

// Column is one custom-field column for a (workspace, object) pair. Whether it
// is active, retired or both is a question of which method returned it, not of
// the type.
//
// The fields carry DIFFERENT disclosure rules, and this is the one place that
// says so, because three surfaces read them:
//
//   - Name and Type are SCHEMA, ambient to any caller who may read records of
//     that object: a field nothing may name is a field a filter cannot use.
//     Ambient is a decision, not an observation — a record payload omits a NULL,
//     so an unused column is not already visible there.
//   - Options is catalogue CONTENT, authored by an admin, and passing it on
//     needs `custom_field:read`.
//
// A Column holds no context and cannot enforce either, so the obligation lands
// on the consumer — written here rather than in each of them.
type Column struct {
	Name string
	Type string
	// Options is a picklist column's allowed values, and is empty for every
	// other type. It travels with the column because a consumer that has to
	// OFFER the field needs them — a builder without them can only ask a reader
	// to type a value from a closed set, which is how a mistyped one becomes a
	// filter that silently matches nothing.
	//
	// The catalogue owns them, as it owns labels: they are per-workspace admin
	// state, not something the engine or a consumer may derive.
	Options []string
}

// Reader answers the active custom-field columns for one core object,
// scoped to the workspace bound to ctx. Implemented by
// modules/customfields' Service; a record store calls it once per
// operation (Get/List/Create/Update) to learn which cf_* columns
// participate, then drives platform/database/storekit's customcolumns.go
// helpers with the result — the store itself never touches the
// custom_field catalog table.
type Reader interface {
	ActiveColumns(ctx context.Context, object string) ([]Column, error)
}

// FilterableReader answers the columns a FILTER may name, which is a different
// question from the ones a write may set: a retired field keeps its column and
// its values, so a saved segment built on it must keep evaluating, while nothing
// may write to it again. It is its own interface rather than a second method on
// Reader because a consumer of one has no use for the other — collections filters
// and never writes cf_* values, and the record stores write and never filter.
type FilterableReader interface {
	FilterableColumns(ctx context.Context, object string) ([]Column, error)
}

// Target is what a custom field may be ATTACHED to.
//
// Its own vocabulary rather than datasource.EntityType, which it resembles.
// Declaring a member there obliges native provider routing, agent record-shape
// generation and every enumerating consumer — held by
// TestTheRecordProviderServesExactlyTheSeamVocabulary — and a contract can carry
// a typed extra field without any of that being true.
//
// Every value the custom_field.object CHECK admits is here, including ones no
// active target list offers: a row written under an older vocabulary must stay
// readable, or fields an installation already configured stop rendering.
type Target string

// The targets themselves. Every value the custom_field.object CHECK admits,
// including the ones no active target list offers today.
const (
	TargetContact      Target = "contact"
	TargetCompany      Target = "company"
	TargetDeal         Target = "deal"
	TargetLead         Target = "lead"
	TargetProject      Target = "project"
	TargetContract     Target = "contract"
	TargetActivity     Target = "activity"
	TargetRelationship Target = "relationship"
	TargetPartner      Target = "partner"
)

// Targets is the closed set, in the order the CHECK constraint spells it so a
// reader comparing the two reads them the same way.
func Targets() []Target {
	return []Target{
		TargetContact, TargetCompany, TargetDeal, TargetLead,
		TargetActivity, TargetProject, TargetRelationship, TargetPartner,
		TargetContract,
	}
}

// Valid reports whether a stored value is one this vocabulary knows. A row
// carrying anything else is a row written by a version this binary cannot
// reason about, and the catalog refuses rather than guesses.
func (t Target) Valid() bool {
	for _, known := range Targets() {
		if t == known {
			return true
		}
	}
	return false
}
