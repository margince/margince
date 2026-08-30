// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The account facts that live in their OWN tables.
//
// Both leaves here reach a child table through a correlated EXISTS, and both
// are multi-valued for the same reason: a company is more than one thing at
// once. It can be a customer and a partner; it can hold the domain it was
// founded under and the one it rebranded to. A column on `organization` could
// hold neither honestly, so the fact lives beside the account and the leaf
// selects accounts that carry AT LEAST the named value.
//
// The rule they share, stated once: a WITHDRAWN row is a fact the account no
// longer has, and the archived predicate lives inside the wrapper so a segment
// naming it stops selecting on something somebody deliberately removed. A leaf
// added here inherits that obligation — TestTheRelationshipLeafExcludesWithdrawnRows
// and TestTheDomainLeafExcludesRemovedRows hold each to it, and to correlating
// to the account it is filtering rather than to nothing.

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// relationshipTypeField is the account's relationship to us, which is
// MULTI-VALUED: an account can be a customer and a partner at once, so the fact
// lives in its own table and this leaf selects accounts that are AT LEAST this.
//
// Archived rows are excluded inside the wrapper. A retired relationship is one
// the account no longer has, and a segment naming it would otherwise keep
// selecting accounts on a fact somebody deliberately withdrew.
//
// A picklist leaf compares text and the engine holds no per-field enum, so a
// value no row carries selects nothing rather than being refused.
// TestAPicklistLeafComparesAnUnrecognisedValueRatherThanRefusingIt gates that.
var relationshipTypeField = storekit.Field{
	Expr:    "rt.relationship_type",
	Type:    storekit.FieldPicklist,
	Options: relationshipTypeValues,
	Link: "EXISTS (SELECT 1 FROM organization_relationship_type rt" +
		" WHERE rt.organization_id = t.id AND rt.archived_at IS NULL AND %s)",
}

// domainField is the account's web domain, which is MULTI-VALUED for the same
// reason its relationship is: a company acquires, rebrands and runs product
// sites, so an account can carry several and the fact lives in its own table.
// The leaf selects accounts that hold AT LEAST this one, and says nothing about
// which is primary — "the account whose domain is acme.com" is the question a
// segment asks, and a primary-only leaf would answer it wrongly for exactly the
// accounts that have more than one.
//
// Archived rows are excluded inside the wrapper, on the relationship leaf's
// reasoning: a withdrawn domain is one the account no longer has, and a segment
// naming it would otherwise keep selecting on a fact somebody deliberately
// removed.
//
// FieldText rather than a picklist: a domain is free text with no enum to
// compare against, and the column is stored lower-cased (org_domain_norm), so a
// caller's value matches the stored form only when they spell it the same way.
// That is the same contract every other text leaf here offers.
var domainField = storekit.Field{
	Expr: "od.domain",
	Type: storekit.FieldText,
	Link: "EXISTS (SELECT 1 FROM organization_domain od" +
		" WHERE od.organization_id = t.id AND od.archived_at IS NULL AND %s)",
}

// The technical facts, which live in `organization_fact` beside every other
// fact the record holds.
//
// Three leaves rather than one, because a reader asks three different
// questions: which system takes their mail, what their site is built with, and
// what they operate. Each reaches the same table under a different `field`, so
// the wrapper pins category and field and the caller's value compares against
// `value_key` alone.
//
// `category = 'signal'` is pinned alongside the field for a reason the field
// alone does not cover: `technology` is a field TWO categories use, and a
// company-stated technology is a different claim from an observed one. Both are
// signals here, so the pin is what keeps a future non-signal `technology` fact
// out of these segments rather than silently widening them.
//
// SOURCE is deliberately not pinned. A person correcting a machine-read value
// rewrites the row's source to `human` (organization_evidence_write.go), so a
// source-constrained leaf would drop exactly the accounts somebody cared enough
// to fix — the opposite of what a segment naming that value means.
//
// No archived predicate, unlike the two leaves above: `organization_fact` has
// no archived_at. A fact the lookup no longer observes is DELETED by its lane's
// reconcile rather than withdrawn, so the rows this sees are the current ones
// by construction.

// technicalFactValue is the column all four technical leaves compare against:
// a fact's value within its field, which is what a segment names.
const technicalFactValue = "tf.value_key"

// technicalFactLink selects accounts holding at least one signal fact of one
// field. The %s is the caller's comparison against value_key.
func technicalFactLink(field string) string {
	return "EXISTS (SELECT 1 FROM organization_fact tf" +
		" WHERE tf.organization_id = t.id AND tf.category = 'signal'" +
		" AND tf.field = '" + field + "' AND %s)"
}

// mailProviderField selects accounts whose mail one named provider receives.
//
// Single-valued in practice — a company has one mail provider, and the lane
// writes it as a one-element replace-set — but reached through the same EXISTS
// as the multi-valued leaves, because the fact lives in the fact table either
// way.
var mailProviderField = storekit.Field{
	Expr:    technicalFactValue,
	Type:    storekit.FieldPicklist,
	Options: mailProviderValues,
	Link:    technicalFactLink("mail_provider"),
}

// hostingProviderField selects accounts hosted with one named provider.
var hostingProviderField = storekit.Field{
	Expr:    technicalFactValue,
	Type:    storekit.FieldPicklist,
	Options: hostingProviderValues,
	Link:    technicalFactLink("hosting_provider"),
}

// operatedServiceField selects accounts that demonstrably operate one named
// service — MULTI-VALUED, and the leaf that answers "every account with a
// webshop".
var operatedServiceField = storekit.Field{
	Expr:    technicalFactValue,
	Type:    storekit.FieldPicklist,
	Options: operatedServiceValues,
	Link:    technicalFactLink("operated_service"),
}

// technologyField selects accounts running one named technology.
//
// FieldText rather than a picklist, and the only one of the four that is: the
// technology vocabulary is the fingerprint ruleset, which grows whenever a rule
// is added, so an enum here would be a second list to keep in step with the
// first and would refuse a value the ruleset had just learned to write.
var technologyField = storekit.Field{
	Expr: technicalFactValue,
	Type: storekit.FieldText,
	Link: technicalFactLink("technology"),
}
