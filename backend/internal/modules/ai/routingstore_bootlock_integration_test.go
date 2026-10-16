// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// A boot-set binding is kept or refused as it stands, so neither a resend nor
// a change asks Google about a location.
func TestALockedVertexBindingIsNeverProbed(t *testing.T) {
	e := setupRateStore(t)
	_, wsCtx := e.seedWorkspace(context.Background(), t)
	ctx := routingWriter(wsCtx)
	boot, err := json.Marshal(vertexRouting("europe-west4"))
	if err != nil {
		t.Fatal(err)
	}
	st, cfg := configtest.StoredOverWith(t, e.pool, Definitions(), ConfigSettings(), "MARGINCE_AI_ROUTING_BINDING="+string(boot))
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{settings: st, cfg: cfg, keys: allCloudKeys(t), selectBrain: selector}

	var locked config.LockedError
	if _, err := store.SetProviderSettings(ctx, providerGeminiVertex, ProviderSettings{Location: "europe-west1"}); !errors.As(err, &locked) {
		t.Errorf("a location move of the locked binding = %v, want the LockedError", err)
	}
	if _, err := store.Replace(ctx, vertexRouting("europe-west4")); err != nil {
		t.Errorf("resending the locked binding = %v, want a no-op", err)
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("Google was asked %d times about a locked binding, want none", n)
	}
}
