// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provider

// Whether a provider can look somebody up at all, asked before spending a
// call on them.
//
// A vendor matches a person BY something: Surfe by a LinkedIn URL, or by a
// name together with a company. A subject carrying neither cannot be found,
// and the vendor says so with a 400 rather than a polite empty answer — one
// that the platform then has to read as a provider fault, because from the
// outside a rejected request and a broken key look the same.
//
// So the rule is declared here as data and checked before the request leaves.
// Descriptor.Identifiers is the sentence a customer reads; MatchRules is the
// same fact in a form the admission pipeline can apply.

// IdentifierField names one member of PersonIdentifiers. It exists so a
// descriptor can state its matching rule as data rather than as prose.
type IdentifierField string

// The fields of PersonIdentifiers, spelled the way a rule names them. Every
// member of the struct has one, and `present` reads each — both held by
// TestEveryIdentifierFieldIsNamedAndUnderstood
// (backend/gates/identifierfields_test.go).
const (
	IdentifierLinkedInURL   IdentifierField = "linkedin_url"
	IdentifierFirstName     IdentifierField = "first_name"
	IdentifierLastName      IdentifierField = "last_name"
	IdentifierCompanyName   IdentifierField = "company_name"
	IdentifierCompanyDomain IdentifierField = "company_domain"
)

// IdentifierFields is every field a rule may name. The registry reads it to
// reject a descriptor naming something that does not exist, which is the one
// way this mechanism fails quietly: an unknown field is never present, so a
// rule carrying a typo can never be satisfied and would refuse every subject.
//
// Held by: TestEveryIdentifierFieldIsNamedAndUnderstood
// (backend/gates/identifierfields_test.go) for the list being complete, and
// TestAProviderCannotMatchOnAnIdentifierNobodyCarries
// (backend/internal/modules/integrations/registrymatch_test.go) for the
// registry actually refusing a rule that names something absent from it.
func IdentifierFields() []IdentifierField {
	return []IdentifierField{
		IdentifierLinkedInURL,
		IdentifierFirstName,
		IdentifierLastName,
		IdentifierCompanyName,
		IdentifierCompanyDomain,
	}
}

// present reports whether one field carries a value.
func (p PersonIdentifiers) present(f IdentifierField) bool {
	switch f {
	case IdentifierLinkedInURL:
		return p.LinkedInURL != ""
	case IdentifierFirstName:
		return p.FirstName != ""
	case IdentifierLastName:
		return p.LastName != ""
	case IdentifierCompanyName:
		return p.CompanyName != ""
	case IdentifierCompanyDomain:
		return p.CompanyDomain != ""
	}
	return false
}

// MatchRule is one combination of identifiers a provider can look somebody up
// by. Every field in AllOf must carry a value; when AnyOf is set, at least one
// of those must too. Two fields that are interchangeable — a company by name
// or by domain — are one rule, not two.
type MatchRule struct {
	AllOf []IdentifierField
	AnyOf []IdentifierField
}

func (r MatchRule) satisfiedBy(p PersonIdentifiers) bool {
	for _, f := range r.AllOf {
		if !p.present(f) {
			return false
		}
	}
	if len(r.AnyOf) == 0 {
		return len(r.AllOf) > 0
	}
	for _, f := range r.AnyOf {
		if p.present(f) {
			return true
		}
	}
	return false
}

// Matchable reports whether these identifiers satisfy any rule — whether the
// provider has anything to look the subject up by.
//
// No rules means matchable by anything. The platform does not invent a
// constraint an adapter did not declare: refusing a lookup the vendor would
// have answered is a worse failure than spending one call to find out.
func (p PersonIdentifiers) Matchable(rules []MatchRule) bool {
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		if r.satisfiedBy(p) {
			return true
		}
	}
	return false
}
