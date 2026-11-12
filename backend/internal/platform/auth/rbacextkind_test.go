// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func registerVisitKind(t *testing.T) {
	t.Helper()
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: "ext_alpha_visit",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
}

func humanWith(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: objects},
	})
}

func TestRequireGatesAnExtensionKindOnItsOwnObjectAndDeniesByDefault(t *testing.T) {
	registerVisitKind(t)
	reader := humanWith(map[string]principal.ObjectGrant{"ext_alpha_visit": {Read: true}})

	if err := Require(reader, "ext:alpha:visit", principal.ActionRead); err != nil {
		t.Errorf("a reader of the kind's object was refused: %v", err)
	}
	for _, action := range []principal.Action{principal.ActionCreate, principal.ActionUpdate, principal.ActionDelete} {
		if err := Require(reader, "ext:alpha:visit", action); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s on a kind granted only read = %v, want ErrPermissionDenied", action, err)
		}
	}
	nobody := humanWith(nil)
	if err := Require(nobody, "ext:alpha:visit", principal.ActionRead); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a role with no grant on the kind = %v, want ErrPermissionDenied", err)
	}
}

func TestRequireRefusesAKindNoUnitDeclaredEvenForTheSystemPrincipal(t *testing.T) {
	registerVisitKind(t)
	system := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	for _, kind := range []string{"ext:beta:visit", "ext:alpha:other", "ext:alpha"} {
		if err := Require(system, kind, principal.ActionRead); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("Require(system, %q) = %v, want ErrPermissionDenied", kind, err)
		}
		granted := humanWith(map[string]principal.ObjectGrant{kind: {Read: true}})
		if err := Require(granted, kind, principal.ActionRead); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("Require(human granted the literal %q) = %v, want ErrPermissionDenied", kind, err)
		}
	}
}

func TestAnExtensionKindIsNotGatedByACoreObjectOfTheSameWords(t *testing.T) {
	registerVisitKind(t)
	core := humanWith(map[string]principal.ObjectGrant{"contact": {Read: true}, "visit": {Read: true}})
	if err := Require(core, "ext:alpha:visit", principal.ActionRead); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a core grant admitted an extension kind: %v", err)
	}
}
