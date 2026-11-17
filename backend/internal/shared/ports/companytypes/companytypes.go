// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package companytypes is the core-internal registry of the company types
// extensions declare. Compose builds one from each composition's units. The
// contacts module asks it whether an `ext:<unit>:<type>` value exists, so it
// never imports a unit or compose.
package companytypes

import (
	"sort"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// Declared is one registered type, as a screen and a refusal need it.
type Declared struct {
	// Value is the stored and wire form, `ext:<unit>:<type>`.
	Value string
	// Unit is the extension that declared it.
	Unit extension.Name
	// Label is what a screen shows.
	Label string
}

// Registry is the company types one composition's units declare. The zero
// value declares none, so a reader that was not handed one refuses every
// extension type instead of accepting it.
type Registry struct {
	byValue map[string]Declared
}

// Of records every unit's types. Two units cannot collide, because the unit
// name is part of the value. A unit declaring a type twice is refused at boot,
// before this runs.
func Of(units []extension.Extension) Registry {
	byValue := map[string]Declared{}
	for _, u := range units {
		for _, t := range u.CompanyTypes {
			value := extension.CompanyTypeValue(u.Name, t.Type)
			byValue[value] = Declared{Value: value, Unit: u.Name, Label: t.Label}
		}
	}
	return Registry{byValue: byValue}
}

// IsExtensionValue reports whether a stored type is an extension's rather than
// one of core's seven.
func IsExtensionValue(value string) bool {
	return strings.HasPrefix(value, extension.CompanyTypePrefix)
}

// Lookup answers the declaration of a value, and false for one no unit declared.
func (r Registry) Lookup(value string) (Declared, bool) {
	d, ok := r.byValue[value]
	return d, ok
}

// All lists every registered type, ordered by value.
func (r Registry) All() []Declared {
	out := make([]Declared, 0, len(r.byValue))
	for _, d := range r.byValue {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}
