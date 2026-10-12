// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One provider's settings change through the real settings store: the row it
// writes, the audit row beside it, and the routing version a serving role's
// watcher compares. None of it is visible without the database.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pinnedBrokerDocument is what a client writes: the broker's host and EU pin on
// the provider, one tier on the product default (no routing) and one opted out
// of it (`{}`).
const pinnedBrokerDocument = `profile: eu_hosted
providers:
  openai_compatible: {base_url: 'https://openrouter.ai/api', upstream: {only: [mistral/eu]}}
tiers:
  cheap_cloud: {provider: openai_compatible, model: mistralai/ministral-8b-2512}
  premium: {provider: openai_compatible, model: mistralai/mistral-small-2603}
  frontier: {provider: openai_compatible, model: mistralai/mistral-small-2603, routing: {}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, dimensions: 1024}
`

// perLaneBrokerRouting is the same binding in the shape every stored row had
// before providers held hosts: each lane names the host and the pin.
const perLaneBrokerRouting = `profile: eu_hosted
tiers:
  cheap_cloud: {provider: openai_compatible, model: mistralai/ministral-8b-2512, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
  premium: {provider: openai_compatible, model: mistralai/mistral-small-2603, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: 'https://openrouter.ai/api', dimensions: 1024, routing: {only: [mistral/eu]}}
`

func providerSettingsAdmin(e *Env) context.Context {
	return e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
}

// plantBrokerRouting stores the document unfinalized, as a PUT delivers it,
// through the writer every routing save goes through.
func plantBrokerRouting(ctx context.Context, t *testing.T, store *ai.RoutingStore) {
	t.Helper()
	var cfg ai.RoutingConfig
	if err := yaml.Unmarshal([]byte(pinnedBrokerDocument), &cfg); err != nil {
		t.Fatalf("the planted binding does not decode: %v", err)
	}
	if _, err := store.Replace(ctx, cfg); err != nil {
		t.Fatalf("storing the planted binding: %v", err)
	}
}

func watchedVersion(t *testing.T, e *Env) string {
	t.Helper()
	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("resolving the stored binding as a serving role does: %v", err)
	}
	return cfg.RoutingVersion()
}

func TestProviderSettings_ABoundHostChangeIsAuditedAndRebindsServingRoles(t *testing.T) {
	e := Setup(t)
	ctx := providerSettingsAdmin(e)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	plantBrokerRouting(ctx, t, store)
	before := watchedVersion(t, e)
	const audits = `SELECT count(*) FROM audit_log WHERE entity_type = 'ai_routing' AND after::text LIKE '%' || $1 || '%'`
	const moved = "https://eu.openrouter.ai/api"

	stored, err := store.SetProviderSettings(ctx, "openai_compatible", ai.ProviderSettings{
		BaseURL: moved, Upstream: &ai.OpenRouterRouting{Provider: ai.OpenRouterProvider{Only: []string{"mistral/eu"}}},
	})
	if err != nil {
		t.Fatalf("SetProviderSettings: %v", err)
	}
	served, err := ai.FromStored(stored, nil)
	if err != nil {
		t.Fatalf("the stored document does not finalize: %v", err)
	}

	if got := served.Tiers[ai.TierPremium].BaseURL; got != moved {
		t.Errorf("served premium host = %q, want the provider's new host", got)
	}
	if n := e.WsCount(t, audits, moved); n != 1 {
		t.Errorf("%d ai_routing audit rows carry the new host, want exactly one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'ai_routing' AND after ? 'ai.routing'`); n < 2 {
		t.Errorf("%d audit rows keyed ai.routing, want the plant and the host change", n)
	}
	if after := watchedVersion(t, e); after == before || after != served.RoutingVersion() {
		t.Errorf("watched version %q → %q (served %q): a serving role would not rebind onto the new host", before, after, served.RoutingVersion())
	}
}

func TestProviderSettings_ClearingABoundHostIsRefusedAndStoresNothing(t *testing.T) {
	e := Setup(t)
	ctx := providerSettingsAdmin(e)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	plantBrokerRouting(ctx, t, store)
	before, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, err = store.SetProviderSettings(ctx, "openai_compatible", ai.ProviderSettings{})

	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || invalid.Code != ai.CodeNoHost {
		t.Fatalf("err = %v, want settings.InvalidValue coded %s", err, ai.CodeNoHost)
	}
	after, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.Revision() != before.Revision() {
		t.Error("a refused provider change still rewrote the stored binding")
	}
}

// The stored row is canonical and read back through JSON: an absent tier
// routing (the product default) and `{}` (none) stay distinct, and the pin
// lives on the provider rather than on any lane.
func TestProviderSettings_TheStoredRowIsCanonical(t *testing.T) {
	e := Setup(t)
	ctx := providerSettingsAdmin(e)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	plantBrokerRouting(ctx, t, store)

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	entry := got.Providers["openai_compatible"]
	if entry.BaseURL != "https://openrouter.ai/api" || entry.Upstream == nil || !slices.Equal(entry.Upstream.Provider.Only, []string{"mistral/eu"}) {
		t.Errorf("provider entry = %+v, want the broker host and the EU pin", entry)
	}
	for tier, lane := range got.Tiers {
		if lane.BaseURL != "" || (lane.Routing != nil && lane.Routing.Provider.Only != nil) {
			t.Errorf("%s stored with host %q routing %+v, want neither on the lane", tier, lane.BaseURL, lane.Routing)
		}
	}
	if r := got.Tiers[ai.TierPremium].Routing; r != nil {
		t.Errorf("premium routing = %+v, want absent kept absent", r)
	}
	if r := got.Tiers[ai.TierFrontier].Routing; r == nil || !r.IsEmpty() {
		t.Errorf("frontier routing = %+v, want `{}` kept", r)
	}
}

// A row written before providers held hosts reads back lifted.
func TestProviderSettings_GetLiftsAStoredOldShapeRow(t *testing.T) {
	e := Setup(t)
	ctx := providerSettingsAdmin(e)
	settingsStore := compose.NewSettingsStore(e.Pool)
	var old ai.RoutingConfig
	if err := yaml.Unmarshal([]byte(perLaneBrokerRouting), &old); err != nil {
		t.Fatalf("the planted binding does not decode: %v", err)
	}
	if err := settings.Set(ctx, settingsStore, ai.Routing, old); err != nil {
		t.Fatalf("planting the old-shape row: %v", err)
	}

	got, err := ai.NewRoutingStore(settingsStore, config.Static(nil)).Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if entry := got.Providers["openai_compatible"]; entry.BaseURL != "https://openrouter.ai/api" || entry.Upstream == nil {
		t.Errorf("provider entry = %+v, want the lanes' host and pin lifted onto it", entry)
	}
	if host := got.Tiers[ai.TierCheapCloud].BaseURL; host != "" {
		t.Errorf("cheap_cloud host = %q after the lift, want it on the provider", host)
	}
}
