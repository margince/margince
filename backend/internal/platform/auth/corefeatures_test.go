// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPublicFeatureBoundaryDoesNotChangeStoredAuthority(t *testing.T) {
	maintenance := grantedCtx("contact", principal.ObjectGrant{Read: true, Create: true, Update: true})
	public := corefeatures.New([]string{"contacts"}).Context(maintenance)
	for name, check := range map[string]func() error{
		"record read": func() error { return Require(public, "contact", principal.ActionRead) },
		"upsert":      func() error { return RequireAny(public, "contact", principal.ActionCreate, principal.ActionUpdate) },
		"scope read":  func() error { return probeRead(public, "contact") },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("disabled contact admitted: %v", err)
			}
		})
	}
	if Allows(public, "contact", principal.ActionRead) {
		t.Fatal("disabled contact offered")
	}
	if err := Require(maintenance, "contact", principal.ActionRead); err != nil {
		t.Fatalf("retained data unavailable to maintenance: %v", err)
	}
	enabled := corefeatures.New(nil).Context(maintenance)
	if err := Require(enabled, "contact", principal.ActionRead); err != nil {
		t.Fatalf("reenabled contact refused: %v", err)
	}
}

func TestRetainedDataAccessStillNeedsItsRoleGrant(t *testing.T) {
	public := corefeatures.New([]string{"contacts"}).Context(grantedCtx("contact", principal.ObjectGrant{Delete: true}))
	retained := corefeatures.RetainedDataContext(public)
	if err := Require(retained, "contact", principal.ActionDelete); err != nil {
		t.Fatal(err)
	}
	if err := Require(retained, "contact", principal.ActionRead); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("retained-data context widened role grants: %v", err)
	}
	if err := Require(public, "contact", principal.ActionDelete); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("retained-data context changed its parent: %v", err)
	}
}
