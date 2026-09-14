// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReadGranted is a pure in-memory check: it reads the acting principal's
// already-merged Permissions and takes no pool connection, so a read that asks
// it per row costs nothing per row.
func TestReadGrantedAdmitsAGrantingRole(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"computed_field": {Read: true}},
		},
	})
	if !auth.ReadGranted(ctx, "computed_field") {
		t.Fatal("want granted for a role that grants computed_field:read")
	}
}

func TestReadGrantedRefusesARoleWithoutTheObject(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			// A role missing the grant entirely — the zero-value ObjectGrant
			// denies, matching Permissions.Allows.
			Objects: map[string]principal.ObjectGrant{"company": {Read: true}},
		},
	})
	if auth.ReadGranted(ctx, "computed_field") {
		t.Fatal("want refused when the role's policy carries no computed_field grant")
	}
}

func TestReadGrantedRefusesWithNoActorBound(t *testing.T) {
	t.Parallel()
	if auth.ReadGranted(context.Background(), "company") {
		t.Fatal("want refused with no actor bound (fail-closed)")
	}
}

func TestReadGrantedTrustsTheSystemPrincipal(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem})
	if !auth.ReadGranted(ctx, "company") {
		t.Fatal("want the system principal trusted by construction, matching Require's own carve-out")
	}
}

// A buyer is refused on the TYPE, not on the accident that one is minted
// carrying no permissions — the distinction Require's refuseBuyer draws. This
// test fails the day a constructor starts minting buyers with a role.
func TestReadGrantedRefusesABuyerEvenWithTheGrant(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalBuyer,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"company": {Read: true}},
		},
	})
	if auth.ReadGranted(ctx, "company") {
		t.Fatal("a Deal Room participant holds no CRM authority, whatever permissions it carries")
	}
}
