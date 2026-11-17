// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestUnattendedSendsWaitWhileDirectedSendsAndUpdatesRun(t *testing.T) {
	ctx := agentCtxWithPassport(ids.NewV7(), principal.ScopeSend, principal.ScopeWrite)
	for _, tc := range []struct {
		name  string
		scope principal.Scope
		mark  func(context.Context) context.Context
		wait  bool
	}{
		{"directed send", principal.ScopeSend, nil, false},
		{"unattended send", principal.ScopeSend, principal.WithUnattended, true},
		{"scheduled run send", principal.ScopeSend, scheduledRun, true},
		{"unattended update", principal.ScopeWrite, principal.WithUnattended, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callCtx := ctx
			if tc.mark != nil {
				callCtx = tc.mark(callCtx)
			}
			spec := mcp.ToolSpec{Name: "action", RequiredScope: tc.scope, Tier: mcp.TierAutoExecute}
			_, err := fullSeatGate().Admit(callCtx, spec, noResolve)
			if tc.wait {
				if !errors.Is(err, apperrors.ErrRequiresApproval) {
					t.Fatalf("unattended send: %v, want approval", err)
				}
			} else if err != nil {
				t.Fatalf("authorised action refused: %v", err)
			}
		})
	}
}

func scheduledRun(ctx context.Context) context.Context {
	return principal.WithAgentRunID(ctx, ids.NewV7())
}
