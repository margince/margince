// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package companytypes is the core-internal registry of the company types
// extensions declare. Compose fills it from the composed set at boot. The
// contacts module asks it whether an `ext:<unit>:<type>` value exists, so it
// never imports a unit or compose.
package companytypes

import (
	"sort"
	"strings"
	"sync"

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

var (
	mu     sync.RWMutex
	byName = map[string]Declared{}
)

// Register records one unit's types. Two units cannot collide, because the unit
// name is part of the value. A unit declaring a type twice is refused at boot,
// before this runs.
func Register(unit extension.Name, types []extension.CompanyType) {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range types {
		value := extension.CompanyTypeValue(unit, t.Type)
		byName[value] = Declared{Value: value, Unit: unit, Label: t.Label}
	}
}

// IsExtensionValue reports whether a stored type is an extension's rather than
// one of core's seven.
func IsExtensionValue(value string) bool {
	return strings.HasPrefix(value, extension.CompanyTypePrefix)
}

// Lookup answers the declaration of a value, and false for one no unit declared.
func Lookup(value string) (Declared, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := byName[value]
	return d, ok
}

// All lists every registered type, ordered by value.
func All() []Declared {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Declared, 0, len(byName))
	for _, d := range byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// Reset empties the registry for a test that composes its own set.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	byName = map[string]Declared{}
}
