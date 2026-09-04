// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The second lock on the governed core-write door.
//
// extcore.go refuses an unattended core write outright, and that refusal is
// tested where it lives. This file holds the OTHER lock, which nothing asserted
// before: the tick's principal carries no permissions, so auth.Require denies it
// every object whether or not that refusal is ever relaxed. The two are
// independent on purpose — a change that spends one should still meet the other.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// tickDeclaration is the declaration the assertions below mint a tick under.
// Its values are the only two the principal reads, so they are named here
// rather than borrowed from a composed unit that could stop declaring a cadence.
func tickDeclaration() extension.JobDeclaration {
	return extension.JobDeclaration{
		Unit:           extension.Name("openchannel"),
		Job:            "drain",
		RequestedScope: extension.Scope("openchannel:drain"),
	}
}

// TestTickPrincipalNamesTheJobAndNoUser pins the shape the tick answers as. The
// absent fields are the assertion: a UserID here would put a person behind work
// nobody requested, and a SeatType would meter one.
func TestTickPrincipalNamesTheJobAndNoUser(t *testing.T) {
	decl := tickDeclaration()
	p := extensionJobPrincipal(decl)

	if p.Type != principal.PrincipalAgent {
		t.Errorf("type = %q, want %q: audit_log.actor_type and the captured_by_kind=agent lane both read it",
			p.Type, principal.PrincipalAgent)
	}
	if want := "agent:" + decl.DispatcherKind(); p.ID != want {
		t.Errorf("id = %q, want %q", p.ID, want)
	}
	if p.UserID != ids.Nil {
		t.Errorf("user id = %s, want none: the tick names a job, not a person", p.UserID)
	}
	if p.SeatType != "" {
		t.Errorf("seat type = %q, want none: no seat is resolved for a tick", p.SeatType)
	}
	if !p.Scopes.Has(principal.Scope(decl.RequestedScope)) {
		t.Errorf("scopes = %v, want the declared %q", p.Scopes, decl.RequestedScope)
	}
}

// TestTickPrincipalIsRefusedEveryObject is the lock itself.
//
// It asserts the PROPERTY rather than a list of object names: an empty grant
// map denies every object in the vocabulary, including the ones a composed
// extension registers at boot, and a list here would fail short the moment the
// vocabulary grew. auth.Require is then called to prove the trapdoor above the
// grant check — PrincipalSystem returns nil in rbac.go before Permissions is
// consulted at all — is not the one this principal takes.
func TestTickPrincipalIsRefusedEveryObject(t *testing.T) {
	p := extensionJobPrincipal(tickDeclaration())

	if len(p.Permissions.Objects) != 0 {
		t.Fatalf("permissions grant %d objects, want none: the empty grant map is what denies every object",
			len(p.Permissions.Objects))
	}
	if p.Type == principal.PrincipalSystem {
		t.Fatal("the tick answers as system, which auth.Require admits before it reads Permissions at all")
	}

	ctx := principal.WithActor(context.Background(), p)
	for _, object := range []string{"person", "deal", "license", "installation_settings"} {
		for _, action := range []principal.Action{
			principal.ActionCreate, principal.ActionRead, principal.ActionUpdate, principal.ActionDelete,
		} {
			if err := auth.Require(ctx, object, action); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("Require(%s.%s) = %v, want permission denied", object, action, err)
			}
		}
	}
}

// TestTickPrincipalLeavesARowOwnerless holds the audit seam's half: the tick is
// an actor storekit accepts, and OwnerOrActor answers nil for it, which is the
// honest owner for a row no person made.
func TestTickPrincipalLeavesARowOwnerless(t *testing.T) {
	p := extensionJobPrincipal(tickDeclaration())
	ctx := principal.WithActor(context.Background(), p)

	actor, err := storekit.Actor(ctx)
	if err != nil {
		t.Fatalf("storekit.Actor: %v", err)
	}
	if actor.ID != p.ID {
		t.Errorf("audited actor id = %q, want %q", actor.ID, p.ID)
	}
	if owner := storekit.OwnerOrActor(ctx, nil); owner != nil {
		t.Errorf("owner = %v, want nil: no person is behind a tick", *owner)
	}
}
