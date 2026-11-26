// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

// preflightLinkTypes refuses a link type the stored value cannot carry, and a
// type one unit declares twice. Two units never collide, because the unit's
// name is part of the value.
func preflightLinkTypes(exts []extension.Extension) error {
	for _, e := range exts {
		seen := make(map[string]bool, len(e.LinkTypes))
		for _, t := range e.LinkTypes {
			if err := t.Validate(); err != nil {
				return fmt.Errorf("compose: extension %q: %w", e.Name, err)
			}
			if seen[t.Type] {
				return fmt.Errorf("compose: extension %q declares link type %q twice", e.Name, t.Type)
			}
			seen[t.Type] = true
		}
	}
	return nil
}

// registerLinkTypes fills the registry the recordlinks module validates
// against, the one the record kinds already live in.
func registerLinkTypes(exts []extension.Extension) {
	for _, e := range exts {
		recordkinds.RegisterLinkTypes(e.Name, e.LinkTypes)
	}
}
