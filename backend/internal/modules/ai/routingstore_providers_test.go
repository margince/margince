// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	brokerHost = "https://openrouter.ai/api"
	gpuHost    = "http://10.0.0.5:8000"
)

// storedOnBroker is a canonical stored document: the broker's host and EU pin
// on the provider, two tiers and the embeddings lane reading them.
func storedOnBroker() RoutingConfig {
	return RoutingConfig{
		Profile: ProfileEUHosted,
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {
			BaseURL: brokerHost, Upstream: &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}}},
		}},
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, Model: "mistralai/mistral-small-2603"},
			TierCheapCloud: {Provider: providerOpenAICompatible, Model: "mistralai/ministral-8b-2512"},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, Model: "mistralai/mistral-embed-2312"}, Dimensions: 1024},
	}
}

// oldClientWrite is what a client that predates providers sends: no providers
// map, and lanes as it last read them.
func oldClientWrite(edit func(tiers map[Tier]ProviderConfig)) RoutingConfig {
	next := storedOnBroker()
	next.Providers = nil
	edit(next.Tiers)
	return next
}

func wantRefusal(t *testing.T, err error, code string, mentions ...string) {
	t.Helper()
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || invalid.Code != code {
		t.Fatalf("err = %v, want settings.InvalidValue with code %s", err, code)
	}
	for _, m := range mentions {
		if !strings.Contains(invalid.Reason, m) {
			t.Errorf("reason %q does not mention %q", invalid.Reason, m)
		}
	}
}

func TestReplace_OldClientEqualHostIsAccepted(t *testing.T) {
	t.Parallel()
	next := oldClientWrite(func(tiers map[Tier]ProviderConfig) {
		lane := tiers[TierPremium]
		lane.BaseURL = "HTTPS://OpenRouter.ai/api/"
		lane.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}, Sort: &OpenRouterSort{By: "throughput"}}}
		tiers[TierPremium] = lane
	})

	stored, _, err := next.replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("an old client re-sending the provider's own host and pins was refused: %v", err)
	}
	premium := stored.Tiers[TierPremium]
	if premium.BaseURL != "" || premium.Routing == nil || premium.Routing.pins() != nil || premium.Routing.Provider.Sort.By != "throughput" {
		t.Errorf("premium stored as %+v (routing %+v), want no host, no pins, and its own sort kept", premium, premium.Routing)
	}
	if got := stored.Providers[providerOpenAICompatible].BaseURL; got != brokerHost {
		t.Errorf("provider host = %q, want the stored spelling %q kept", got, brokerHost)
	}
}

// A tier's host filter is never lifted onto a connection that has none: it
// decides who may read every tier's requests, so a write sets it on the
// connection or not at all. The host still lifts, as it always did.
func TestReplace_ATierPinIsRefusedWhenTheConnectionHasNone(t *testing.T) {
	t.Parallel()
	current := storedOnBroker()
	current.Providers = nil
	next := oldClientWrite(func(tiers map[Tier]ProviderConfig) {
		for tier, lane := range tiers {
			lane.BaseURL = brokerHost
			lane.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}}}
			tiers[tier] = lane
		}
	})

	_, _, err := next.replacing(current)

	wantConnectionKeyRefusal(t, err, "tiers.cheap_cloud.routing.provider.only")
}

// wantConnectionKeyRefusal asserts err refuses path as a key that belongs on
// the connection, with the code that sends the admin to the provider sheet.
func wantConnectionKeyRefusal(t *testing.T, err error, path string) {
	t.Helper()
	var faults routingFaults
	if !errors.As(err, &faults) {
		t.Fatalf("err = %v, want a refusal per path", err)
	}
	for _, f := range faults {
		if f.Path == path && f.Code == CodeMovedToProvider && strings.Contains(f.Message, "set on the connection") {
			return
		}
	}
	t.Fatalf("faults %+v do not refuse %s as a connection key", faults, path)
}

func TestReplace_OldClientDifferentHostIsMovedToProvider(t *testing.T) {
	t.Parallel()
	next := oldClientWrite(func(tiers map[Tier]ProviderConfig) {
		lane := tiers[TierPremium]
		lane.BaseURL = "https://api.mistral.ai"
		tiers[TierPremium] = lane
	})

	_, _, err := next.replacing(storedOnBroker())

	wantRefusal(t, err, CodeMovedToProvider, "tier premium", "PUT /ai/provider-settings/openai_compatible")
}

func TestReplace_OldClientDifferentPinsAreMovedToProvider(t *testing.T) {
	t.Parallel()
	next := oldClientWrite(func(tiers map[Tier]ProviderConfig) {
		lane := tiers[TierCheapCloud]
		lane.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"nebius/eu"}}}
		tiers[TierCheapCloud] = lane
	})

	_, _, err := next.replacing(storedOnBroker())

	wantConnectionKeyRefusal(t, err, "tiers.cheap_cloud.routing.provider.only")
}

// Pins are a set of hosts: the same hosts in another order are the provider's.
func TestReplace_OldClientPinsInAnotherOrderAreAccepted(t *testing.T) {
	t.Parallel()
	current := storedOnBroker()
	current.Providers[providerOpenAICompatible].Upstream.Provider.Only = []string{"mistral/eu", "nebius/eu"}
	next := oldClientWrite(func(tiers map[Tier]ProviderConfig) {
		lane := tiers[TierPremium]
		lane.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"nebius/eu", "mistral/eu"}}}
		tiers[TierPremium] = lane
	})

	if _, _, err := next.replacing(current); err != nil {
		t.Fatalf("the provider's pins in another order were refused: %v", err)
	}
}

// The decisions lane's endpoint is reconciled the way a tier host is.
func TestReplace_OldClientDecisionsEndpointIsReconciled(t *testing.T) {
	t.Parallel()
	const endpoint = "https://openrouter.ai/api/alpha/decisions"
	current := storedOnBroker()
	current.Providers[providerJevCompatible] = ProviderSettings{BaseURL: endpoint}
	current.Decisions = &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-1.13"}

	same := current
	same.Providers = nil
	same.Decisions = &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-1.13", BaseURL: endpoint + "/"}
	if _, _, err := same.replacing(current); err != nil {
		t.Fatalf("the provider's own endpoint re-sent on the decisions lane was refused: %v", err)
	}

	moved := same
	moved.Decisions = &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-1.13", BaseURL: "http://127.0.0.1:8767/v1/systemone"}
	_, _, err := moved.replacing(current)
	wantRefusal(t, err, CodeMovedToProvider, "the decisions lane", "jev_compatible")
}

func TestReplace_PersistsCanonical(t *testing.T) {
	t.Parallel()
	served, err := storedOnBroker().finalize()
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	stored, _, err := served.replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("writing back the served binding: %v", err)
	}
	for tier, lane := range stored.Tiers {
		if lane.BaseURL != "" || lane.Routing.pins() != nil {
			t.Errorf("%s stored as host %q routing %+v, want neither the host nor the pins on the lane", tier, lane.BaseURL, lane.Routing)
		}
	}
	if e := stored.Embeddings; e.BaseURL != "" || e.Routing != nil {
		t.Errorf("embeddings stored as host %q routing %+v, want it reading its provider", e.BaseURL, e.Routing)
	}
}

// A write of the document as stored stores it unchanged: the ETag a client was
// handed is still the stored revision afterwards, and no tier gains the product
// default it did not write.
func TestReplace_ANoOpWriteKeepsTheRevision(t *testing.T) {
	t.Parallel()
	before := storedOnBroker().canonical()

	stored, _, err := before.replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}

	if stored.Revision() != before.Revision() {
		t.Errorf("a no-op write moved the revision from %s to %s", before.Revision(), stored.Revision())
	}
	for tier, lane := range stored.Tiers {
		if lane.Routing != nil {
			t.Errorf("%s stored routing %+v, want none: it wrote none", tier, lane.Routing)
		}
	}
}

// An absent tier `routing` means the product default and `{}` means none; a
// write stores each as written rather than freezing the default into it.
func TestReplace_AbsentUpstreamSurvivesARoundTrip(t *testing.T) {
	t.Parallel()
	stored, served, err := storedOnBroker().replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if r := stored.Tiers[TierPremium].Routing; r != nil {
		t.Errorf("an absent routing was stored as %+v", r)
	}
	if r := stored.canonical().Tiers[TierPremium].Routing; r != nil {
		t.Errorf("an absent routing reads back as %+v", r)
	}
	if r := served.Tiers[TierPremium].Routing; r == nil || r.Provider.Sort.By == "" {
		t.Errorf("served premium routing = %+v, want the product default under the provider's pin", r)
	}
}

func TestReplace_EmptyUpstreamSurvivesARoundTrip(t *testing.T) {
	t.Parallel()
	next := storedOnBroker()
	lane := next.Tiers[TierPremium]
	lane.Routing = &OpenRouterRouting{}
	next.Tiers[TierPremium] = lane

	stored, served, err := next.replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if r := stored.canonical().Tiers[TierPremium].Routing; r == nil || !r.IsEmpty() {
		t.Errorf("`{}` reads back as %+v, want `{}`", r)
	}
	if r := served.Tiers[TierPremium].Routing; r == nil || r.Provider.Sort != nil || !slices.Equal(r.Provider.Only, []string{"mistral/eu"}) {
		t.Errorf("served premium routing = %+v, want the provider's pin and no default", r)
	}
}

func TestReplace_WriteWithoutProvidersKeepsStoredPins(t *testing.T) {
	t.Parallel()
	next := oldClientWrite(func(map[Tier]ProviderConfig) {})

	stored, served, err := next.replacing(storedOnBroker())
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if up := stored.Providers[providerOpenAICompatible].Upstream; up == nil || !slices.Equal(up.Provider.Only, []string{"mistral/eu"}) {
		t.Errorf("provider upstream = %+v after a write that sent no providers, want the stored pin kept", up)
	}
	if r := served.Embeddings.Routing; r == nil || !slices.Equal(r.Provider.Only, []string{"mistral/eu"}) {
		t.Errorf("served embeddings routing = %+v, want the stored pin", r)
	}
}

// A client that sends the providers map owns it, so `{}` clears every entry —
// and a lane left bound to a provider that needs a host is then refused as such.
func TestReplace_WriteWithEmptyProvidersMapClearsUnboundEntries(t *testing.T) {
	t.Parallel()
	current := storedOnBroker()
	current.Providers[providerOllama] = ProviderSettings{BaseURL: gpuHost}

	native := current
	native.Providers = map[string]ProviderSettings{}
	native.Tiers = map[Tier]ProviderConfig{TierPremium: {Provider: providerOllama, Model: "qwen3"}}
	native.Embeddings = EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOllama, Model: "bge-m3"}, Dimensions: 1024}
	native.Profile = ProfileSovereign
	stored, _, err := native.replacing(current)
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if stored.Providers != nil {
		t.Errorf("providers = %+v after writing `{}`, want every entry cleared", stored.Providers)
	}

	unhosted := current
	unhosted.Providers = map[string]ProviderSettings{}
	_, _, err = unhosted.replacing(current)
	wantRefusal(t, err, CodeNoHost, "openai_compatible is bound by tier cheap_cloud, tier premium, the embeddings lane")
}

func TestSetProviderSettings_ClearingABoundHostIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := storedOnBroker().withProviderSettings(providerOpenAICompatible, ProviderSettings{})

	wantRefusal(t, err, CodeNoHost, "openai_compatible is bound by tier cheap_cloud, tier premium, the embeddings lane", "rebind")
}

// The embeddings lane at a server of its own does not need its provider's host.
func TestSetProviderSettings_AnEmbeddingsServerNeedsNoProviderHost(t *testing.T) {
	t.Parallel()
	current := storedOnBroker()
	current.Profile = ProfileCloudFrontier
	current.Tiers = map[Tier]ProviderConfig{TierPremium: {Provider: providerGemini, Model: "gemini-3.1-flash-lite"}}
	current.Embeddings.BaseURL = gpuHost

	_, served, err := current.withProviderSettings(providerOpenAICompatible, ProviderSettings{})
	if err != nil {
		t.Fatalf("clearing the host of a provider only an embeddings server binds: %v", err)
	}
	if got := served.Embeddings.BaseURL; got != gpuHost {
		t.Errorf("served embeddings host = %q, want its own server %q", got, gpuHost)
	}
}

func TestSetProviderSettings_BoundHostChangeMovesTheVersion(t *testing.T) {
	t.Parallel()
	before, err := storedOnBroker().finalize()
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	_, served, err := storedOnBroker().withProviderSettings(providerOpenAICompatible, ProviderSettings{BaseURL: "https://eu.openrouter.ai/api", Upstream: &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}}}})
	if err != nil {
		t.Fatalf("changing the bound host: %v", err)
	}
	if served.RoutingVersion() == before.RoutingVersion() {
		t.Error("a bound provider's new host left the routing version alone; no serving role would rebind")
	}
	if got := served.Tiers[TierPremium].BaseURL; got != "https://eu.openrouter.ai/api" {
		t.Errorf("served premium host = %q, want the provider's new host", got)
	}
}

func TestSetProviderSettings_UnboundEntrySavesShapeOnly(t *testing.T) {
	t.Parallel()
	before, err := storedOnBroker().finalize()
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	stored, served, err := storedOnBroker().withProviderSettings(providerOllama, ProviderSettings{BaseURL: gpuHost})
	if err != nil {
		t.Fatalf("setting an unbound provider's host: %v", err)
	}
	if got := stored.Providers[providerOllama].BaseURL; got != gpuHost {
		t.Errorf("stored ollama host = %q, want %q", got, gpuHost)
	}
	if served.RoutingVersion() != before.RoutingVersion() {
		t.Error("an entry no lane binds moved the routing version")
	}
	if stored.Revision() == before.Revision() {
		t.Error("an entry no lane binds left the revision alone; a concurrent editor would not see it")
	}

	cleared, _, err := stored.withProviderSettings(providerOllama, ProviderSettings{})
	if err != nil {
		t.Fatalf("removing the entry: %v", err)
	}
	if _, kept := cleared.Providers[providerOllama]; kept {
		t.Error("a zero entry was stored rather than removed")
	}

	_, _, err = storedOnBroker().withProviderSettings(providerOllama, ProviderSettings{BaseURL: "gpu-box"})
	wantRefusal(t, err, settings.CodeInvalidValue, "ollama", "not an http(s) URL")
}

// An installation that binds nothing can still have a host set ahead of the
// binding; only the entry's shape is checked.
func TestSetProviderSettings_AnUnconfiguredInstallationTakesAHost(t *testing.T) {
	t.Parallel()
	stored, _, err := RoutingConfig{}.withProviderSettings(providerOpenAICompatible, ProviderSettings{BaseURL: brokerHost})
	if err != nil {
		t.Fatalf("withProviderSettings: %v", err)
	}
	if err := validateStoredRouting(stored); err != nil {
		t.Fatalf("the entry validator refused a host set before any binding: %v", err)
	}
	if err := validateStoredRouting(RoutingConfig{Providers: map[string]ProviderSettings{"nope": {}}}); err == nil {
		t.Error("an unknown provider entry on an unbound document was accepted")
	}
}

func TestSetProviderSettings_DeniedWithoutUpdate(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{routingSettingsObject: {Read: true}}},
	})

	_, err := NewRoutingStore(nil, nil).SetProviderSettings(ctx, providerOpenAICompatible, ProviderSettings{BaseURL: brokerHost})

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied for a reader without update", err)
	}
}

func TestSetProviderSettings_UnknownProviderIsNotFound(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{routingSettingsObject: {Read: true, Update: true}}},
	})

	_, err := NewRoutingStore(nil, nil).SetProviderSettings(ctx, "not_a_provider", ProviderSettings{BaseURL: brokerHost})

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for a provider this build does not know", err)
	}
}

func TestGet_LiftsAStoredOldShapeRow(t *testing.T) {
	t.Parallel()
	old := storedOnBroker()
	old.Providers = nil
	for tier, lane := range old.Tiers {
		lane.BaseURL, lane.Routing = brokerHost, &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}, Sort: &OpenRouterSort{By: "throughput"}}}
		old.Tiers[tier] = lane
	}
	old.Embeddings.BaseURL = brokerHost

	got := old.canonical()

	if entry := got.Providers[providerOpenAICompatible]; entry.BaseURL != brokerHost || entry.Upstream == nil {
		t.Errorf("provider entry = %+v, want the lanes' host and pin lifted onto it", entry)
	}
	if lane := got.Tiers[TierPremium]; lane.BaseURL != "" || lane.Routing == nil || lane.Routing.Provider.Sort.By != "throughput" || lane.Routing.pins() != nil {
		t.Errorf("premium = %+v (routing %+v), want only its serving preferences left on it", lane, lane.Routing)
	}
}

// A canonical document lanes read their host from is lifted with nothing to
// say: an empty lane host is the provider's, not a second opinion on it.
func TestLift_ACanonicalDocumentLiftsSilently(t *testing.T) {
	t.Parallel()
	log, buf := warnings()

	storedOnBroker().liftLaneProviderFields(log)

	if buf.Len() != 0 {
		t.Errorf("lifting a canonical document warned: %s", buf)
	}
}
