// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The filter VOCABULARY: what a field is, what it may point at, and the types
// that decide both. Split from the compiler beside it because they are two
// subjects with different readers — this one is what every surface that
// OFFERS a filter reads (the vocabulary endpoint, the builders behind it),
// while the compiler is read by nobody but the engine.

// FieldType types a filterable field (features/10 §3: typed operators
// per field type). It decides which operators apply and how a leaf's
// value is validated before it may become a bind parameter.
type FieldType string

// The filterable field types. Six of them are what a custom field may be; the
// last two are core-only, because the catalogue offers neither.
const (
	FieldText     FieldType = "text"
	FieldNumber   FieldType = "number"
	FieldDate     FieldType = "date"
	FieldCurrency FieldType = "currency"
	FieldPicklist FieldType = "picklist"
	FieldBoolean  FieldType = "boolean"
	// FieldID covers the allow-list's UUID reference columns (owner_id,
	// stage_id, …): equality/membership only, value must parse as a UUID
	// so a malformed id fails validation (422), never query execution.
	FieldID FieldType = "id"
	// FieldDomain is a host column, and it is a type rather than text because
	// the operand is NORMALIZED before it binds: a caller who pasted
	// `https://www.acme.example/careers` out of an email signature is asking
	// about `acme.example`, which is what the column holds.
	//
	// The company LIST already folds its `domain` parameter that way, so
	// without a type here one product fact would answer two different questions
	// depending on which surface asked — a saved view returning nothing for the
	// URL the list matches.
	//
	// Folding is the one place this engine touches an operand, and the type is
	// what keeps that bounded: every other value still reaches SQL exactly as it
	// arrived. A per-field transform would have bought the same behaviour by
	// making arbitrary rewriting possible everywhere, which is the guarantee
	// that makes "selects nothing" safe rather than merely unhelpful.
	FieldDomain FieldType = "domain"
)

// Field is one entry of a resource's closed filter vocabulary: the API
// name maps to a fixed SQL expression (table alias included, e.g.
// "t.owner_id") plus its type. Only expressions from this map ever
// reach the query text.
type Field struct {
	Expr string
	Type FieldType
	// Link makes this a correlated-subquery leaf rather than a base-table
	// one: Expr names the column INSIDE the subquery, and Link is the
	// EXISTS template — exactly one %s — the compiled comparison is
	// substituted into. It exists because some filterable facts are link
	// rows rather than columns (a tag lives in the polymorphic taggable
	// join), and a link row is present or absent where a column is null
	// or not. Empty for every base-table field.
	Link string
	// References names the record type this field's ids point at, for a
	// surface that has to offer the record rather than ask for its uuid.
	//
	// The compiler has no use for it — an id compares as an id whatever it
	// refers to — so it sits here for one reason: this is where a field is
	// declared, and a lookup table keyed by field name elsewhere would be a
	// second list of the engine's fields for a new leaf to fall out of.
	//
	// It is required of every id field in a vocabulary that is PUBLISHED to a
	// client — the collections segment engines, gated by
	// TestEveryIDFieldDeclaresWhatItReferences — and left empty everywhere
	// else. An engine built to answer a count nobody reads a field list from
	// (automation's preview vocabularies) owes no target, because nothing can
	// offer a picker for it.
	References Reference
	// Options is a picklist field's allowed values, for a surface that has to
	// OFFER them. Empty for every other type, and empty for a picklist whose
	// values this engine does not know.
	//
	// ADVERTISEMENT only: compileLeaf does not refuse a value outside the set, and
	// TestAPicklistLeafComparesAnUnrecognisedValueRatherThanRefusingIt holds that
	// so the behaviour is gated rather than assumed. Refusing would be a live-API
	// change — a saved segment holding a value since removed from its set would
	// begin failing at read time — which is why the set travels first and the
	// refusal is a separate call to make.
	//
	// What this fixes meanwhile is the surface: a builder that knows the values
	// offers them instead of asking a reader to type one, which is how a typo
	// became a filter that matched nothing and read as a settled answer.
	Options []string
}

// Reference is a record type an id field's values point at. Named rather than a
// bare string so an unlisted target cannot be assigned, the same way FieldType
// closes the type column above it.
type Reference string

// The record types a FieldID field may reference. The values are the contract's
// own record-type words (`app_user`, not `user`), so a client keying a picker on
// them needs no translation table.
const (
	RefTag      Reference = "tag"
	RefAppUser  Reference = "app_user"
	RefTeam     Reference = "team"
	RefCompany  Reference = "company"
	RefPipeline Reference = "pipeline"
	RefStage    Reference = "stage"
	RefProject  Reference = "project"
)

// ReferenceTargets is every target the engine admits, and the ONE list of them.
//
// A function beside the constants rather than a slice restated in each test that
// needs it — the shape fieldcatalog.Types() already uses for the same reason. The
// point is which drift stays catchable: a constant absent here fails the sweep
// over the engines, and an entry here absent from the contract's enum fails the
// parity gate in compose. Restating this set in either test would make both gates
// pass on a stale copy of it, which is the one failure they exist to catch.
func ReferenceTargets() []Reference {
	return []Reference{
		RefTag, RefAppUser, RefTeam, RefCompany,
		RefPipeline, RefStage, RefProject,
	}
}
