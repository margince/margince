// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"strings"
)

// ReportNamePrefix opens the name of every count and breakdown a unit adds.
// A name then shows at a glance that it is not one of core's.
const ReportNamePrefix = "ext:"

// ReportCount is a number a unit adds to Reports and Home: how many live
// records a population holds. It is inert data. Core runs it through the same
// report engine as its own counts. Row scope, field masks and the group floor
// apply as they do there.
type ReportCount struct {
	// Name is the short name, `[a-z][a-z0-9_]*`, at most 32 characters.
	Name string
	// Label is what a screen shows, in English.
	Label string
	// Population is what is counted: the short name of a record kind this unit
	// declares, or the key of one of core's deal populations. Core keys carry a
	// hyphen and a kind name never does, so the two cannot be confused.
	Population string
}

// ReportBreakdown is a count split by one field, added to Reports and Home.
// Like a ReportCount it names data and never SQL. The field is a column the
// unit declared on its own kind, or a custom field of a core population. Core
// renders the only query that reads it.
type ReportBreakdown struct {
	// Name is the short name, `[a-z][a-z0-9_]*`, at most 32 characters.
	Name string
	// Label is what a screen shows, in English.
	Label string
	// Population is as for ReportCount.
	Population string
	// By is the field the count is split by. On a record kind of this unit it
	// names one of the kind's declared fields. On a core population it names a
	// custom field as the population's vocabulary spells it: `cf_<name>` for the
	// deal's own, `company_cf_<name>` for its company's. A multi-select field
	// splits by the combination of values a record holds. One record is then in
	// one group, and the groups add up to the whole.
	By string
}

// ReportName is the stored and wire form of a unit's count or breakdown. It is
// the one place the form is built, so a unit can only name one under its own
// name.
func ReportName(unit Name, name string) string {
	return ReportNamePrefix + string(unit) + ":" + name
}

// ValidatePopulation says whether a population names a core population, which
// compose checks against core's catalog, or is one of the unit's own kinds.
// It returns the kind's short name for the second, and "" for the first.
func ValidatePopulation(population string, kinds []RecordKindDeclaration) (kind string, err error) {
	if strings.Contains(population, "-") {
		return "", nil
	}
	for _, k := range kinds {
		if k.Kind == population {
			return k.Kind, nil
		}
	}
	return "", fmt.Errorf("population %q is neither a record kind this unit declares nor a core population", population)
}

// Validate refuses a count core could not carry.
func (c ReportCount) Validate(kinds []RecordKindDeclaration) error {
	return validateReport(c.Name, c.Label, c.Population, kinds)
}

// Validate refuses a breakdown core could not carry. A field of a record kind
// must be one the kind declares. A field of a core population is checked
// against that installation's custom fields when the answer is built.
func (b ReportBreakdown) Validate(kinds []RecordKindDeclaration) error {
	if err := validateReport(b.Name, b.Label, b.Population, kinds); err != nil {
		return err
	}
	kind, _ := ValidatePopulation(b.Population, kinds)
	if kind == "" {
		if !recordKindGrammar.MatchString(b.By) {
			return fmt.Errorf("report breakdown %q splits by %q, which is not a field name", b.Name, b.By)
		}
		return nil
	}
	for _, k := range kinds {
		if k.Kind != kind {
			continue
		}
		for _, f := range k.Fields {
			if f.Name == b.By {
				return nil
			}
		}
	}
	return fmt.Errorf("report breakdown %q splits by %q, which record kind %q does not declare", b.Name, b.By, kind)
}

func validateReport(name, label, population string, kinds []RecordKindDeclaration) error {
	if !recordKindGrammar.MatchString(name) || len(name) > maxRecordKindLength {
		return fmt.Errorf("report %q is not a valid name (a lower-case letter, then lower-case letters, digits and underscores, at most %d characters)", name, maxRecordKindLength)
	}
	if strings.TrimSpace(label) == "" {
		return fmt.Errorf("report %q has no label", name)
	}
	if _, err := ValidatePopulation(population, kinds); err != nil {
		return fmt.Errorf("report %q: %w", name, err)
	}
	return nil
}
