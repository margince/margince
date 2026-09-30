// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRequireAdmin(t *testing.T) {
	admin := principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{RoleKeys: []string{"member", "admin"}}}
	nonAdmin := principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{RoleKeys: []string{"member"}}}
	system := principal.Principal{Type: principal.PrincipalSystem, ID: "system"}

	if err := RequireAdmin(principal.WithActor(context.Background(), admin)); err != nil {
		t.Fatalf("admin denied: %v", err)
	}
	if err := RequireAdmin(principal.WithActor(context.Background(), system)); err != nil {
		t.Fatalf("system denied: %v", err)
	}
	if err := RequireAdmin(principal.WithActor(context.Background(), nonAdmin)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("non-admin: want ErrPermissionDenied, got %v", err)
	}
	if err := RequireAdmin(context.Background()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("no principal: want ErrPermissionDenied, got %v", err)
	}
}

// Coaching follows the team_lead grant, never a role key: a custom role holding
// it coaches, and a seeded lead's key without it does not.
func TestRequireCoachFollowsTheTeamLeadGrant(t *testing.T) {
	coaching := map[string]principal.ObjectGrant{objTeamLead: {Create: true, Read: true}}
	readingOnly := map[string]principal.ObjectGrant{objTeamLead: {Read: true}}
	seat := func(kind principal.PrincipalType, key string, objects map[string]principal.ObjectGrant) context.Context {
		return principal.WithActor(context.Background(), principal.Principal{
			Type: kind, ID: "seat:test",
			Permissions: principal.Permissions{RoleKeys: []string{key}, Objects: objects},
		})
	}
	cases := []struct {
		name  string
		ctx   context.Context
		admit bool
	}{
		{"custom role holding the grant", seat(principal.PrincipalHuman, "custom_regional_lead", coaching), true},
		{"seeded lead without the grant", seat(principal.PrincipalHuman, "manager", nil), false},
		{"read without create", seat(principal.PrincipalHuman, "custom_regional_lead", readingOnly), false},
		{"agent holding the grant", seat(principal.PrincipalAgent, "manager", coaching), false},
		{"buyer holding the grant", seat(principal.PrincipalBuyer, "manager", coaching), false},
		{"system", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system"}), false},
	}
	for _, tc := range cases {
		err := RequireCoach(tc.ctx)
		if tc.admit && err != nil {
			t.Errorf("%s: refused: %v", tc.name, err)
		}
		if !tc.admit && !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: want ErrPermissionDenied, got %v", tc.name, err)
		}
	}
}
