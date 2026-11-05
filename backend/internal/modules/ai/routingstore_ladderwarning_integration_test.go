// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// A saved routing document is warned about as a boot would warn about it.
// The warning reads the document the process will serve, not the stored one.
func TestASavedRoutingWhoseLaddersReachOneAPIIsWarnedAbout(t *testing.T) {
	e := setupRateStore(t)
	_, wsCtx := e.seedWorkspace(context.Background(), t)
	ctx := routingWriter(wsCtx)
	st, cfg := configtest.StoredOverWith(t, e.pool, Definitions(), ConfigSettings())
	var heard bytes.Buffer
	log := slog.New(slog.NewTextHandler(&heard, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store := &RoutingStore{settings: st, cfg: cfg, keys: allCloudKeys(t), log: log}

	one := collapsedOnBroker("m")
	save := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Tiers:      map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one},
		Embeddings: EmbeddingsConfig{ProviderConfig: collapsedOnBroker("embed")},
	}
	if _, err := store.Replace(ctx, save); err != nil {
		t.Fatalf("saving the routing: %v", err)
	}
	if out := heard.String(); !strings.Contains(out, "ai: routing:") || !strings.Contains(out, "model=m") {
		t.Fatalf("a save whose ladders reach one API was not warned about: %q", out)
	}
}
