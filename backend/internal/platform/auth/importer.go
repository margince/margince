// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Who may write the importer's source_system namespace.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importRunObject is migration.ImportRunObject, spelled as a literal because
// platform/auth may not import a module. auth/importer_test.go asserts the two
// spellings are equal, so a rename over there cannot quietly turn this door
// into "always false" — which would not fail a test, it would refuse every
// import.
const importRunObject = "import_run"

// ImporterObject names the object DeclaredImporter asks about, so a test
// outside this package can hold the literal against migration.ImportRunObject.
// It is exported for that pin alone: platform/auth cannot import the module
// that owns the constant, and a silent divergence refuses every import without
// failing anything.
func ImporterObject() string { return importRunObject }

// DeclaredImporter reports whether the caller may write the importer's
// source_system namespace: a HUMAN holding import_run:create, the same grant
// the product's own import surface takes (compose/csvimport.go).
//
// A GRANT, not a role. import_run is admin/ops-only on every verb by seeded
// policy — "a workspace-wide bulk mutation of the estate" — so asking for the
// grant says what the caller is doing rather than who they are.
//
// RequireAdmin would be wrong on both halves. It admits PrincipalSystem, and
// TestTheSystemPrincipalDoesNotUnlockTheImporterNamespace exists to keep the
// automation engine out of this namespace; and an agent carries its granting
// human's whole Permissions, so an admin's agent would inherit the door.
//
// The type check comes FIRST and must stay first: Require returns nil for the
// system principal before it ever looks at grants, so an Allows call reached
// first would admit it.
func DeclaredImporter(ctx context.Context) bool {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman {
		return false
	}
	return Allows(ctx, importRunObject, principal.ActionCreate)
}
