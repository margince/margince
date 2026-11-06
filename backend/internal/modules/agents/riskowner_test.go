// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"github.com/margince/margince/backend/internal/platform/auth"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAttentionViewsPassTheOwnerToTheirSharedQueries(t *testing.T) {
	owner := ids.NewV7()
	var slippingOwner, riskOwner *ids.UUID
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterSlippingTools(r, func(_ context.Context, _ int, selected *ids.UUID) ([]SlippingDeal, error) {
		slippingOwner = selected
		return nil, nil
	}, nil, testStalledDays, testStalledDays)
	RegisterNetworkTools(r, nil, nil, nil, func(_ context.Context, selected *ids.UUID) (AtRiskReport, error) {
		riskOwner = selected
		return AtRiskReport{}, nil
	})
	RegisterCommitmentTool(r, func(context.Context, CommitmentQuery) (CommitmentSweep, error) { return CommitmentSweep{}, nil })
	registerWhereServed(r, readAttention{})
	for _, view := range []string{"slipping", "at_risk"} {
		args := json.RawMessage(`{"view":"` + view + `","owner_id":"` + owner.String() + `"}`)
		if _, err := r.InvokeServed(scopedCtx(principal.ScopeRead), readAttentionToolName, args); err != nil {
			t.Fatalf("%s: %v", view, err)
		}
	}
	for view, got := range map[string]*ids.UUID{"slipping": slippingOwner, "at_risk": riskOwner} {
		if got == nil || *got != owner {
			t.Errorf("%s owner = %v, want %s", view, got, owner)
		}
	}
}
