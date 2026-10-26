// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A unit declares default grants per system role, and generation refuses a key
// that names none. The published surface cannot import identity, so
// extension.SystemRoleKeys is a declared mirror of the roles identity seeds.
// This holds it true both ways. A seeded role not mirrored cannot get a
// default. A mirrored key that is no role passes generation and fails every boot.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestTheExtensionRoleKeysMirrorTheSeededRoles(t *testing.T) {
	t.Parallel()
	seeded := identity.SystemRoleKeys()
	for _, key := range seeded {
		if !slices.Contains(extension.SystemRoleKeys, key) {
			t.Errorf("identity seeds role %q, which extension.SystemRoleKeys lacks: a unit cannot declare a default for it", key)
		}
	}
	for _, key := range extension.SystemRoleKeys {
		if !slices.Contains(seeded, key) {
			t.Errorf("extension.SystemRoleKeys names %q, which identity does not seed: generation would accept a default the boot refuses", key)
		}
	}
}
