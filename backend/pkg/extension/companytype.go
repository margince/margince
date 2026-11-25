// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"regexp"
	"strings"
)

// CompanyTypePrefix opens every company type an extension declares, so a stored
// value says at a glance that it is not one of core's seven. It is
// QualifiedPrefix.
const CompanyTypePrefix = QualifiedPrefix

// companyTypeGrammar is the type half of CompanyTypeNamePattern.
var companyTypeGrammar = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// CompanyType is one thing a unit says a company can be, beside core's own
// types. It is declared by its short name and stored as CompanyTypeValue, so two
// units can each declare `reseller` without sharing the word.
type CompanyType struct {
	// Type is the short name: lower-case [a-z0-9] segments joined by single
	// underscores.
	Type string
	// Label is what a screen shows for the type, in English.
	Label string
}

// Validate refuses a type a stored value or a screen could not carry.
func (t CompanyType) Validate() error {
	if !companyTypeGrammar.MatchString(t.Type) {
		return fmt.Errorf("company type %q is not a valid type name (lower-case [a-z0-9] segments joined by single underscores)", t.Type)
	}
	if strings.TrimSpace(t.Label) == "" {
		return fmt.Errorf("company type %q has no label", t.Type)
	}
	return nil
}

// CompanyTypeValue is the stored and wire form of a unit's type. It is the one
// place the form is built, so a unit can only ever name a value under its own
// name.
func CompanyTypeValue(unit Name, typ string) string {
	return QualifiedName(unit, typ)
}
