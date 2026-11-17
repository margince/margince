// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUnattendedAgentsCannotApproveAnything(t *testing.T) {
	actor := principal.Principal{Type: principal.PrincipalAgent, UserID: ids.NewV7(), Scopes: principal.NewScopeSet(principal.ScopeWrite, principal.ScopeSend)}
	kinds := []string{"advance_deal"}
	for kind := range sendingKinds {
		kinds = append(kinds, kind)
	}
	for _, kind := range kinds {
		held := row{Kind: kind}
		if err := agentMayDecide(context.Background(), actor, held, true, ownReleaseRefused); err != nil {
			t.Fatalf("attended approval of %s: %v", kind, err)
		}
		for _, ctx := range []context.Context{principal.WithUnattended(context.Background()), principal.WithAgentRunID(context.Background(), ids.NewV7())} {
			if err := agentMayDecide(ctx, actor, held, true, ownReleaseAllowed); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("unattended approval of %s: %v", kind, err)
			}
			if err := agentMayDecide(ctx, actor, held, false, ownReleaseRefused); err != nil {
				t.Fatalf("rejection of %s: %v", kind, err)
			}
		}
	}
}
