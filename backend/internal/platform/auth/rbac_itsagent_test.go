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
)

// An agent on a colleague's passport passes where its colleague does;
// automatic work with no passport, and a buyer, do not.
func TestRequireHumanOrItsAgentAdmitsAnAgentOnlyForAUser(t *testing.T) {
	user, passport := ids.New[ids.UserKind]().UUID, ids.New[ids.PassportKind]().UUID
	for name, tc := range map[string]struct {
		p     principal.Principal
		admit bool
	}{
		"a human":                   {principal.Principal{Type: principal.PrincipalHuman, ID: "human:x", UserID: user}, true},
		"an agent for a user":       {principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x", UserID: user, OnBehalfOf: user, PassportID: passport}, true},
		"an agent with no passport": {principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x", UserID: user, OnBehalfOf: user}, false},
		"a buyer":                   {principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:x"}, false},
	} {
		err := RequireHumanOrItsAgent(principal.WithActor(context.Background(), tc.p))
		if tc.admit != (err == nil) || (err != nil && !errors.Is(err, apperrors.ErrPermissionDenied)) {
			t.Errorf("%s: err = %v, want admitted %v", name, err, tc.admit)
		}
	}
}
