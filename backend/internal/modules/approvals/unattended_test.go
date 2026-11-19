// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
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

// The expiry sweep decides approvals in bulk under its own system name. A
// caller with no actor, or any other actor, is refused before a row is read.
func TestOnlyTheExpiryClockMayExpireApprovals(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"no actor": context.Background(),
		"another system actor": principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalSystem, ID: "system:other",
		}),
	} {
		if _, err := (&Service{}).ExpireDue(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: ExpireDue = %v, want ErrPermissionDenied", name, err)
		}
	}
}

// A colleague's own request is read only by that colleague or the agent on
// their passport. An unattended system actor has no request of its own.
func TestAnOwnRequestIsReadOnlyByItsColleague(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:worker",
	})
	_, err := OwnRequestForTargetTx(ctx, nil, approvalsubject.KindOwnershipTransfer, "deal", ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a system actor reading an own request answered %v, want ErrPermissionDenied", err)
	}
}
