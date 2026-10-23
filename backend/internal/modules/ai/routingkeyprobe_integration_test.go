// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// A keyless binding is refused by the SAVE, not only by the check behind it.
//
// A save naming no gemini_vertex lane takes a path of its own and asks Google
// nothing. That is the path every reported case took, a jev or an anthropic
// tier. A check reached only from the Vertex probe passes its own unit tests
// and refuses none of them.
func TestSavingAKeylessBindingIsRefusedWithNoVertexLaneInSight(t *testing.T) {
	e := setupRateStore(t)
	_, wsCtx := e.seedWorkspace(context.Background(), t)
	ctx := routingWriter(wsCtx)

	anthropicEnv := cloudKeyEnv[providerAnthropic]
	if anthropicEnv == "" {
		t.Fatal("anthropic is not a key-bearing provider, so this case proves nothing")
	}
	// Every key but the one the save needs.
	keys := map[string]string{}
	for provider, env := range cloudKeyEnv {
		if provider != providerAnthropic {
			keys[env] = "k"
		}
	}
	st, cfg := configtest.StoredOverWith(t, e.pool, Definitions(), ConfigSettings())
	store := &RoutingStore{settings: st, cfg: cfg, keys: config.Static(keys)}

	next := RoutingConfig{
		Profile: ProfileEUHosted,
		Tiers: map[Tier]ProviderConfig{
			TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOllama, Model: "bge-m3"}},
	}

	_, err := store.ReplaceIfVersion(ctx, next, "")
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("the save answered %v, want a refusal naming the missing key", err)
	}
	if !strings.Contains(invalid.Reason, anthropicEnv) {
		t.Errorf("the refusal reads %q, which does not name %s", invalid.Reason, anthropicEnv)
	}
	if !strings.Contains(invalid.Reason, "tier local_small") {
		t.Errorf("the refusal reads %q, which does not name the lane", invalid.Reason)
	}

	// And nothing was stored, so the screen cannot report a binding that is
	// not running.
	if got, err := store.Get(ctx); err == nil && got.Tiers[TierLocalSmall].Provider == providerAnthropic {
		t.Error("the refused binding was stored anyway")
	}
}
