// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The reported shape: a save naming a keyless vendor is accepted, and the
// router keeps serving the previous binding. So the status read has to name
// the lane that cannot run, and the variable to set.
func TestTheStatusNamesTheLaneWhoseVendorHoldsNoKey(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"ai_routing":     {Read: true, Update: true},
			"ai_diagnostics": {Read: true},
			"ai_budget":      {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	// Parsed and stored through the real writer, so the document under test is
	// one an admin could have saved.
	cfg, err := ai.ParseRouting([]byte(`profile: cloud_frontier
tiers:
  cheap_cloud: {provider: anthropic, model: claude-sonnet-5}
embeddings: {provider: fake, model: fake-embed}
`))
	if err != nil {
		t.Fatal(err)
	}
	routing := ai.NewRoutingStore(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	if _, err := routing.ReplaceIfVersion(ctx, cfg, ""); err != nil {
		t.Fatalf("the save was refused, so the state this reports on cannot arise: %v", err)
	}

	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), StoredConfig(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))
	status, err := store.ReadStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if status.LanesNeedingAKey == nil {
		t.Fatal("the status names no lane, so a reader cannot tell the saved binding is not running")
	}
	var found bool
	for _, lane := range *status.LanesNeedingAKey {
		if lane.Lane != "cheap_cloud" {
			continue
		}
		found = true
		if lane.Provider != "anthropic" || lane.KeyEnv != "ANTHROPIC_API_KEY" {
			t.Errorf("lane = %s/%s needing %q, want anthropic needing ANTHROPIC_API_KEY",
				lane.Lane, lane.Provider, lane.KeyEnv)
		}
	}
	if !found {
		t.Errorf("lanes = %+v, want the tier whose vendor holds no key", *status.LanesNeedingAKey)
	}
	// The embeddings lane runs on the fake, which takes no key.
	for _, lane := range *status.LanesNeedingAKey {
		if lane.Lane == "embed" {
			t.Errorf("the fake embeddings lane is reported as needing %q", lane.KeyEnv)
		}
	}
}
