// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent with no passport, or no colleague behind one, acts for nobody, and
// the refusal is answered from the principal before any database read.
func TestAnAgentActingForNobodyReachesNoMemberRoute(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"no principal": context.Background(),
		"a passportless agent": principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:policy", OnBehalfOf: ids.NewV7(),
		}),
		"a human": principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:u1", UserID: ids.NewV7(),
		}),
	} {
		if _, err := (&Service{}).agentsUser(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: agentsUser = %v, want ErrPermissionDenied", name, err)
		}
	}
}
