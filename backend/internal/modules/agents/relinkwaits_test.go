// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func passportWriter() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:own", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		SeatType: principal.SeatFull, Permissions: everyRecordedGrant(), Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
}

// admitRelink runs one relink spec through the production gate, its destination
// read as the registry's resolver reads it, with no record version observed.
func admitRelink(t *testing.T, spec mcp.ToolSpec, entityType string) error {
	t.Helper()
	gate := auth.NewGate(unboundedAuthority{})
	args := json.RawMessage(`{"entity_type":"` + entityType + `"}`)
	_, err := gate.Admit(passportWriter(), spec, func() (mcp.TierResolverInput, error) {
		return mcp.TierResolverInput{Args: args}, nil
	})
	return err
}

// Filing under a project sets a retention floor only a member can undo. So it
// waits for a human even when the agent acts as its own user. Every other
// destination is that user's own move and runs at once.
func TestRelinkToAProjectWaitsEvenForItsUser(t *testing.T) {
	for _, spec := range []mcp.ToolSpec{relinkActivity{}.Spec(), relinkActivities{}.Spec()} {
		if err := admitRelink(t, spec, "project"); !errors.Is(err, apperrors.ErrRequiresApproval) {
			t.Errorf("%s to a project, for its user's own agent: %v, want it to wait for a human", spec.Name, err)
		}
		if err := admitRelink(t, spec, "deal"); err != nil {
			t.Errorf("%s to a deal, for its user's own agent: %v, want it to run at once", spec.Name, err)
		}
	}
}
