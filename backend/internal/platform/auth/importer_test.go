// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Who the importer door lets through, and — the part that matters — who it
// does not.
//
// The namespace this door guards is a security boundary: the activity and lead
// stores key their idempotent replay on (source_system, source_id), so anyone
// able to spell the prefix can plant a row under a guessed record id and have a
// later import hand it back as already landed.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importerGrant is import_run:create — what the product's own import surface
// takes, and the only grant this door reads.
func importerGrant() principal.Permissions {
	return principal.Permissions{Objects: map[string]principal.ObjectGrant{
		"import_run": {Create: true},
	}}
}

func TestDeclaredImporterAdmitsOnlyAHumanHoldingTheGrant(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    principal.Principal
		want bool
		why  string
	}{
		{
			name: "a human holding import_run:create",
			p:    principal.Principal{Type: principal.PrincipalHuman, ID: "human:admin", Permissions: importerGrant()},
			want: true,
			why:  "this is the importer; refusing it refuses every import",
		},
		{
			name: "a human without the grant",
			p:    principal.Principal{Type: principal.PrincipalHuman, ID: "human:rep"},
			want: false,
			why:  "a rep may not declare themselves an import",
		},
		{
			// An agent carries its granting human's WHOLE Permissions, so the
			// grant check alone would admit an admin's agent. The type check is
			// the only thing keeping it out.
			name: "an agent carrying the same grant",
			p: principal.Principal{
				Type: principal.PrincipalAgent, ID: "agent:assistant",
				OnBehalfOf: ids.NewV7(), Permissions: importerGrant(),
			},
			want: false,
			why:  "an agent inherits its human's grants; the importer is not a thing it may become",
		},
		{
			// Require returns nil for the system principal BEFORE it looks at
			// grants, so a door that asked Allows first would admit this one.
			// TestTheSystemPrincipalDoesNotUnlockTheImporterNamespace holds the
			// same line at the activities provider.
			name: "the system principal",
			p:    principal.Principal{Type: principal.PrincipalSystem, ID: "system:engine"},
			want: false,
			why:  "the automation engine stamps its own reserved names and never the importer's",
		},
		{
			name: "a connector",
			p:    principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", Permissions: importerGrant()},
			want: false,
			why:  "a capture connector writes its own provenance, not an import's",
		},
		{
			name: "a buyer",
			p:    principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:outside", Permissions: importerGrant()},
			want: false,
			why:  "an external contact in a deal room is the last principal that may forge provenance",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), tc.p)
			if got := DeclaredImporter(ctx); got != tc.want {
				t.Errorf("DeclaredImporter = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

// A request carrying no principal at all is not an importer. Without the `ok`
// check this would read the zero Principal, whose Type is "" and not human —
// so it is refused either way, and this pins that rather than leaving it to
// the zero value staying what it is.
func TestDeclaredImporterRefusesAnUnauthenticatedContext(t *testing.T) {
	if DeclaredImporter(context.Background()) {
		t.Error("a context with no actor was admitted as the importer")
	}
}
