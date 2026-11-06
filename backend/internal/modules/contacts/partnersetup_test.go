// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent setting the tier is refused for who it is, as the other agent limits are.
func TestAnAgentSettingAMarginTierIsForbidden(t *testing.T) {
	tier := "tier1_15"
	ctx := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test"})
	var refusal *httperr.DetailedError
	err := requireMarginTierGrant(ctx, UpsertPartnerInput{MarginTier: &tier})
	if !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden || refusal.Code != "agent_sets_no_margin_tier" {
		t.Errorf("setting the tier answered %v", err)
	}
}
