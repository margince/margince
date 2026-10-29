// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"slices"
	"strings"
)

// LinkEnds are the records a typed link joins, in the words the link table
// stores. A link type narrows them; it cannot widen them.
var LinkEnds = []string{"deal", "company", "contact"}

// LinkType is one way a unit says two core records relate, beside core's own
// types. It is declared by its short name and stored as LinkTypeValue, so two
// units can each declare `follow_up_of` without sharing the word.
type LinkType struct {
	// Type is the short name, `[a-z][a-z0-9_]*`, at most 32 characters.
	Type string
	// Label is what a screen shows for the type, in English.
	Label string
	// From and To name the records the link may start and end on, from
	// LinkEnds. An empty list allows all of them.
	From, To []string
}

// Validate refuses a type a stored value or a screen could not carry.
func (t LinkType) Validate() error {
	if !recordKindGrammar.MatchString(t.Type) || len(t.Type) > maxRecordKindLength {
		return fmt.Errorf("link type %q is not a valid type name (a lower-case letter, then lower-case letters, digits and underscores, at most %d characters)", t.Type, maxRecordKindLength)
	}
	if strings.TrimSpace(t.Label) == "" {
		return fmt.Errorf("link type %q has no label", t.Type)
	}
	for _, end := range slices.Concat(t.From, t.To) {
		if !slices.Contains(LinkEnds, end) {
			return fmt.Errorf("link type %q names %q as an end; the ends are %s", t.Type, end, strings.Join(LinkEnds, ", "))
		}
	}
	return nil
}

// LinkTypeValue is the stored and wire form of a unit's type. It is the one
// place the form is built, so a unit can only ever name a value under its own
// name.
func LinkTypeValue(unit Name, typ string) string {
	return RecordKindPrefix + string(unit) + ":" + typ
}
