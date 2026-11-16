// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
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
		name       string
		scope      principal.Scope
		unattended bool
		wait       bool
	}{
		{"directed send", principal.ScopeSend, false, false},
		{"unattended send", principal.ScopeSend, true, true},
		{"unattended update", principal.ScopeWrite, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callCtx := ctx
			if tc.unattended {
				callCtx = principal.WithUnattended(callCtx)
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
