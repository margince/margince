// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordkinds is the core-internal registry of the record kinds
// extensions declare. Compose fills it from the composed set at boot. Modules
// and platform code ask it whether an `ext:<unit>:<kind>` name is a record.
// They also ask where its rows live, so none of them imports a unit or compose.
package recordkinds

import (
	"sort"
	"strings"
	"sync"

	"github.com/margince/margince/backend/pkg/extension"
)

// Declared is one registered kind, as a gate, a screen and a refusal need it.
type Declared struct {
	// Name is the stored and wire form, `ext:<unit>:<kind>`.
	Name string
	// Unit is the extension that declared it.
	Unit extension.Name
	// Kind is the short name inside the unit.
	Kind string
	// DisplayName is what a screen calls one record of the kind.
	DisplayName string
	// Table is the unit's own table, in the ext schema.
	Table string
	// Object is the RBAC object that gates every action on the kind.
	Object string
	// Fields are the values a record carries.
	Fields []extension.RecordKindField
	// NaturalKey names the fields that together identify one record.
	NaturalKey []string
	// DeletePolicy is never empty: the default is filled in at registration.
	DeletePolicy extension.DeletePolicy
}

var (
	mu     sync.RWMutex
	byName = map[string]Declared{}
)

// Register records one unit's kinds. Two units cannot collide, because the unit
// name is part of the name. A unit declaring a kind twice is refused at boot,
// before this runs.
func Register(unit extension.Name, kinds []extension.RecordKindDeclaration) {
	mu.Lock()
	defer mu.Unlock()
	for _, k := range kinds {
		name := extension.RecordKindName(unit, k.Kind)
		policy := k.DeletePolicy
		if policy == "" {
			policy = extension.DeleteArchiveOnly
		}
		byName[name] = Declared{
			Name: name, Unit: unit, Kind: k.Kind, DisplayName: k.DisplayName, Table: k.Table,
			Object: extension.RecordKindObject(unit, k.Kind), Fields: k.Fields,
			NaturalKey: k.NaturalKey, DeletePolicy: policy,
		}
	}
}

// IsExtensionName reports whether a stored kind is an extension's rather than
// one of core's. It says nothing about whether any unit declared it.
func IsExtensionName(name string) bool {
	return strings.HasPrefix(name, extension.RecordKindPrefix)
}

// Lookup answers the declaration of a name, and false for one no unit declared.
func Lookup(name string) (Declared, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := byName[name]
	return d, ok
}

// All lists every registered kind, ordered by name.
func All() []Declared {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Declared, 0, len(byName))
	for _, d := range byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Reset empties the registry for a test that composes its own set.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	byName = map[string]Declared{}
}
