// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The refusal comes before the transaction is touched, so a nil one proves the
// seed wrote nothing.
func TestOnlyTheSystemSeedsAWorkspacePipeline(t *testing.T) {
	store := &Store{}
	for name, actor := range map[string]principal.Principal{
		"an unbounded admin": {
			Type: principal.PrincipalHuman, ID: "human:admin", UserID: ids.NewV7(),
			Permissions: principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll},
		},
		"an agent": {Type: principal.PrincipalAgent, ID: "agent:seed"},
	} {
		ctx := principal.WithActor(context.Background(), actor)
		if err := store.SeedDefaultsTx(ctx, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s seeding the default pipeline: %v, want permission denied", name, err)
		}
		open := []StageSeed{{Name: "Qualified", WinProbability: 10}}
		if err := store.SeedPipelineTx(ctx, nil, "Sales", open); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s seeding a configured pipeline: %v, want permission denied", name, err)
		}
	}
}
