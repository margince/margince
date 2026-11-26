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
	// Timeline is nil for a kind whose rows do not enter a History.
	Timeline *extension.RecordKindTimeline
	// View is the path segment under the unit's screen that shows one record,
	// or empty for the screen itself.
	View string
}

// TitleColumn is the field a record of the kind is titled and searched by. It
// is the History label field, else the first field of the natural key. Search
// matches on it, and the readiness check asks for the index that match needs.
func (d Declared) TitleColumn() string {
	if d.Timeline != nil {
		return d.Timeline.LabelField
	}
	return d.NaturalKey[0]
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
			NaturalKey: k.NaturalKey, DeletePolicy: policy, Timeline: k.Timeline, View: k.View,
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

// DeclaredBy reports whether the unit declared the kind. A kind another unit
// declared, or none did, answers false.
func DeclaredBy(unit string, name string) bool {
	declared, ok := Lookup(name)
	return ok && string(declared.Unit) == unit
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
	resetLinkTypes()
}
