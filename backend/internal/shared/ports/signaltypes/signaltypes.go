// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package signaltypes is the core-internal registry of the signal types
// extensions declare. Compose fills it from the composed set at boot. The
// signals store checks evidence against it. The erasure path asks it which
// evidence members hold personal data. Neither imports a unit or compose.
package signaltypes

import (
	"strings"
	"sync"

	"github.com/margince/margince/backend/pkg/extension"
)

// Declared is one registered signal type.
type Declared struct {
	// Name is the stored and wire form, `ext:<unit>:<kind>`.
	Name string
	// Unit is the extension that declared it.
	Unit extension.Name
	extension.SignalTypeDeclaration
}

var (
	mu     sync.RWMutex
	byName = map[string]Declared{}
)

// Register records one unit's signal types. Two units cannot collide, because
// the unit name is part of the name.
func Register(unit extension.Name, types []extension.SignalTypeDeclaration) {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range types {
		name := extension.SignalKindName(unit, t.Kind)
		byName[name] = Declared{Name: name, Unit: unit, SignalTypeDeclaration: t}
	}
}

// IsExtensionName reports whether a stored signal kind is an extension's rather
// than one of core's. It says nothing about whether any unit declared it.
func IsExtensionName(name string) bool {
	return strings.HasPrefix(name, extension.SignalKindPrefix)
}

// Lookup answers the declaration of a name, and false for one no unit declared.
func Lookup(name string) (Declared, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := byName[name]
	return d, ok
}
