// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAUserOrTheirAttendedSendingAgentReleasesAHeldMessage(t *testing.T) {
	agent := func(scopes ...principal.Scope) principal.Principal {
		return principal.Principal{Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(), Scopes: principal.NewScopeSet(scopes...)}
	}
	attended := context.Background()
	scheduledRun := principal.WithAgentRunID(context.Background(), ids.NewV7())
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		actor   principal.Principal
		release bool
	}{
		{"user", attended, principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()}, true},
		{"attended agent with send", attended, agent(principal.ScopeWrite, principal.ScopeSend), true},
		{"attended agent with write only", attended, agent(principal.ScopeWrite), false},
		{"unattended agent with send", principal.WithUnattended(attended), agent(principal.ScopeWrite, principal.ScopeSend), false},
		{"scheduled run with send", scheduledRun, agent(principal.ScopeWrite, principal.ScopeSend), false},
		{"system", attended, principal.Principal{Type: principal.PrincipalSystem}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := mayReleaseHeld(principal.WithActor(tc.ctx, tc.actor))
			if tc.release && err != nil {
				t.Fatalf("release refused: %v", err)
			}
			if !tc.release && !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("release = %v, want ErrPermissionDenied", err)
			}
		})
	}
	if err := mayReleaseHeld(context.Background()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("no caller: %v, want ErrPermissionDenied", err)
	}
}
