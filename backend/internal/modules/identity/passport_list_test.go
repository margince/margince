// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A zero Service has no pool, so a refusal here is proven to come before any
// query.
func TestListPassportsRefusesAnAgentAndABuyer(t *testing.T) {
	for name, caller := range map[string]principal.Principal{
		"agent": {Type: principal.PrincipalAgent, ID: "agent:night"},
		"buyer": {Type: principal.PrincipalBuyer, ID: "buyer:deal-room"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), caller)
			_, err := (&Service{}).ListPassports(ctx, Identity{})
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("ListPassports as %s = %v, want ErrPermissionDenied", name, err)
			}
		})
	}
}
