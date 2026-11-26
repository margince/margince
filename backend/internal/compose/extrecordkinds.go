// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

// preflightKindsLinksSignals refuses a unit's record kinds, link types or signal
// types that core could not carry. The rules are the unit's own
// (Extension.ValidateDeclarations), so a unit's tests meet the boot's refusal.
// Two units never collide, because the unit's name is part of every stored
// name. validateExtensionSet already refuses two units whose namespaces open
// one another, the only way two kinds could share an RBAC object.
func preflightKindsLinksSignals(exts []extension.Extension) error {
	for _, e := range exts {
		if err := e.ValidateDeclarations(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
	}
	return nil
}

// registerRecordKinds fills the registry the platform and modules validate
// against.
func registerRecordKinds(exts []extension.Extension) {
	registerLinkTypes(exts)
	for _, e := range exts {
		recordkinds.Register(e.Name, e.RecordKinds)
	}
}

// withRecordKindRbac adds the declared kinds' objects and starting grants to
// those the verbs derived. An object a verb already declared is not added
// twice, and the joined grants are validated as one set.
func withRecordKindRbac(exts []extension.Extension, objects []identity.RbacObject, defaults []identity.RbacDefault) ([]identity.RbacObject, []identity.RbacDefault, error) {
	kindObjects, kindDefaults := recordKindRbac(exts)
	for _, o := range kindObjects {
		if !slices.Contains(objects, o) {
			objects = append(objects, o)
		}
	}
	defaults = append(defaults, kindDefaults...)
	if err := identity.ValidateRbacDefaults(defaults...); err != nil {
		return nil, nil, fmt.Errorf("compose: %w", err)
	}
	return objects, defaults, nil
}

// recordKindRbac derives the RBAC objects and starting grants of the declared
// kinds. A kind that declares no defaults gets an object and no grant, so every
// role is denied every action until an admin ticks one.
func recordKindRbac(exts []extension.Extension) ([]identity.RbacObject, []identity.RbacDefault) {
	var objects []identity.RbacObject
	var defaults []identity.RbacDefault
	for _, e := range exts {
		for _, k := range e.RecordKinds {
			object := identity.RbacObject(extension.RecordKindObject(e.Name, k.Kind))
			objects = append(objects, object)
			for _, d := range k.Defaults {
				actions := make([]string, 0, len(d.Actions))
				for _, a := range d.Actions {
					actions = append(actions, string(a))
				}
				defaults = append(defaults, identity.RbacDefault{Object: object, Role: d.Role, Actions: actions})
			}
		}
	}
	return objects, defaults
}
