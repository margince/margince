// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A routing document written before hosts and upstream preferences moved onto
// the provider, in the two shapes installations store: an EU broker binding
// pinned on every lane, and a frontier binding with a capped tier and a
// decisions lane. The versions are literals taken from the code that wrote
// these documents, so a move that re-attributed every cached brief fails here.
func TestDigest_UnchangedBindingKeepsItsVersion(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"an EU broker binding pinned on every lane": {
			doc: `profile: eu_hosted
tiers:
  cheap_cloud: {provider: openai_compatible, model: mistralai/ministral-14b-2512, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu], require_parameters: true}}
  premium: {provider: openai_compatible, model: mistralai/mistral-medium-3-5, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu], require_parameters: true}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: "https://openrouter.ai/api", dimensions: 1024, routing: {only: [mistral/eu]}}
`,
			want: "cb4a84809fd2c0d0c466c60019c640d0bd9fc0ec5cdbfbe2a48f23600ac475e3",
		},
		"a frontier binding with a capped tier and a decisions lane": {
			doc: `profile: cloud_frontier
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
  cheap_cloud: {provider: openai_compatible, model: openai/gpt-oss-120b, base_url: "https://openrouter.ai/api", routing: {sort: throughput, quantizations: [bf16], reasoning_effort: low}}
  premium: {provider: openai_compatible, model: anthropic/claude-sonnet-4.5, base_url: "https://openrouter.ai/api", routing: {sort: throughput, quantizations: [bf16]}}
embeddings: {provider: openai_compatible, model: baai/bge-m3, base_url: "https://openrouter.ai/api"}
decisions: {provider: jev_compatible, model: typesafe/jev-1.13, base_url: "https://openrouter.ai/api/alpha/decisions"}
`,
			want: "498572ba4efcfbfd7e27eaeda5f259192d1cc99596e65cda4366ecbf86dfdfa6",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := versionOf(t, tc.doc); got != tc.want {
				t.Errorf("RoutingVersion = %s, want %s: the binding is unchanged, so every cached brief must stay attributed to it", got, tc.want)
			}
		})
	}
}

const broker = "https://openrouter.ai/api"

// warnings collects the Warn records a lift writes, as JSON lines.
func warnings() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})), &buf
}

func warnLines(buf *bytes.Buffer) []string {
	return slices.DeleteFunc(strings.Split(buf.String(), "\n"), func(line string) bool { return line == "" })
}

func brokerLane(model string, routing *OpenRouterRouting) ProviderConfig {
	return ProviderConfig{Provider: providerOpenAICompatible, Model: model, BaseURL: broker, Routing: routing}
}

func mustParse(t *testing.T, doc string) RoutingConfig {
	t.Helper()
	cfg, err := ParseRouting([]byte(doc))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	return cfg
}

func TestLift_SingleLaneHostMovesToProvider(t *testing.T) {
	log, _ := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://api.mistral.ai"}}}
	lifted := cfg.liftLaneProviderFields(log)
	if got := lifted.Providers[providerOpenAICompatible].BaseURL; got != "https://api.mistral.ai" {
		t.Errorf("provider host = %q, want the lane's", got)
	}
	if got := lifted.canonical().Tiers[TierCheapCloud].BaseURL; got != "" {
		t.Errorf("canonical lane host = %q, want none: the provider holds it", got)
	}
	if got := cfg.Tiers[TierCheapCloud].BaseURL; got == "" {
		t.Error("the lift cleared the caller's own lane; it must work on a copy")
	}
}

func TestLift_EquivalentSpellingsAreOneHost(t *testing.T) {
	log, buf := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://openrouter.ai/api/"},
		TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://OpenRouter.ai/api"},
	}}
	lifted := cfg.liftLaneProviderFields(log)
	if got := lifted.Providers[providerOpenAICompatible].BaseURL; got != "https://openrouter.ai/api/" {
		t.Errorf("provider host = %q, want the first lane's spelling", got)
	}
	if lines := warnLines(buf); len(lines) != 0 {
		t.Errorf("two spellings of one host warned: %v", lines)
	}
}

func TestLift_DisagreeingLanesFirstInOrderWinsAndWarns(t *testing.T) {
	log, buf := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://b.example"},
		TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://a.example"},
	}}
	lifted := cfg.liftLaneProviderFields(log)
	if got := lifted.Providers[providerOpenAICompatible].BaseURL; got != "https://a.example" {
		t.Errorf("provider host = %q, want tier cheap_cloud's, the first by name", got)
	}
	lines := warnLines(buf)
	if len(lines) != 1 || !strings.Contains(lines[0], `"dropped":"tier premium"`) || !strings.Contains(lines[0], `"kept_from":"tier cheap_cloud"`) {
		t.Errorf("warnings = %v, want one naming the kept and the dropped lane", lines)
	}
}

func TestLift_ReasoningEffortStaysOnTheTier(t *testing.T) {
	log, _ := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium: brokerLane("m", &OpenRouterRouting{Sort: SortThroughput, ReasoningEffort: effortHigh}),
	}}
	lifted := cfg.liftLaneProviderFields(log)
	if got := lifted.Providers[providerOpenAICompatible].Upstream; !reflect.DeepEqual(got, &OpenRouterRouting{Sort: SortThroughput}) {
		t.Errorf("provider upstream = %+v, want the sort without the effort", got)
	}
	if got := lifted.Tiers[TierPremium].ReasoningEffort; got != effortHigh {
		t.Errorf("tier effort = %q, want %q", got, effortHigh)
	}
}

func TestLift_IsIdempotent(t *testing.T) {
	log, _ := warnings()
	cfg := RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers: map[Tier]ProviderConfig{
			TierCheapCloud: brokerLane("m", &OpenRouterRouting{Sort: SortThroughput, ReasoningEffort: effortLow}),
			TierPremium:    brokerLane("m", nil),
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: brokerLane("e", &OpenRouterRouting{Ignore: []string{"x"}})},
		Decisions:  &DecisionsConfig{Provider: providerJevCompatible, Model: "d", BaseURL: exampleBrokerDecisionEndpoint},
	}
	once := cfg.liftLaneProviderFields(log)
	if twice := once.liftLaneProviderFields(log); !reflect.DeepEqual(once, twice) {
		t.Errorf("lift(lift(x)) = %+v, want lift(x) = %+v", twice, once)
	}
}

func TestResolve_EveryLaneReadsItsProvider(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api"}
  jev_compatible: {base_url: "https://openrouter.ai/api/alpha/decisions"}
tiers:
  premium: {provider: openai_compatible, model: m}
embeddings: {provider: openai_compatible, model: e}
decisions: {provider: jev_compatible, model: d}
`)
	if cfg.Tiers[TierPremium].BaseURL != broker || cfg.Embeddings.BaseURL != broker {
		t.Errorf("chat lanes resolved to %q and %q, want the provider's host", cfg.Tiers[TierPremium].BaseURL, cfg.Embeddings.BaseURL)
	}
	if cfg.Decisions.BaseURL != exampleBrokerDecisionEndpoint {
		t.Errorf("decisions lane resolved to %q, want its provider's endpoint", cfg.Decisions.BaseURL)
	}
}

func TestResolve_EmbeddingsGetsOnlyFilterFields(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {only: [a], sort: throughput, quantizations: [fp8], allow_fallbacks: false}}
tiers:
  premium: {provider: openai_compatible, model: m}
embeddings: {provider: openai_compatible, model: e}
`)
	off := false
	if got, want := cfg.Embeddings.Routing, (&OpenRouterRouting{Only: []string{"a"}, AllowFallbacks: &off}); !reflect.DeepEqual(got, want) {
		t.Errorf("embeddings routing = %+v, want only the host filters %+v", got, want)
	}
	full := &OpenRouterRouting{Only: []string{"a"}, Sort: SortThroughput, Quantizations: []string{"fp8"}, AllowFallbacks: &off}
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, full) {
		t.Errorf("tier routing = %+v, want the whole block %+v", got, full)
	}
}

func TestResolve_AbsentUpstreamStillGetsProductDefault(t *testing.T) {
	const doc = `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api"}
tiers:
  premium: {provider: openai_compatible, model: m}
  cheap_cloud: {provider: openai_compatible, model: m, reasoning_effort: low}
embeddings: {provider: gemini, model: e}
`
	cfg := mustParse(t, doc)
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, DefaultOpenRouterRouting()) {
		t.Errorf("tier premium routing = %+v, want the product default", got)
	}
	capped := DefaultOpenRouterRouting()
	capped.ReasoningEffort = effortLow
	if got := cfg.Tiers[TierCheapCloud].Routing; !reflect.DeepEqual(got, capped) {
		t.Errorf("tier cheap_cloud routing = %+v, want the product default capped at low", got)
	}
}

func TestResolve_EmptyUpstreamMeansNoPreferences(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {}}
tiers:
  premium: {provider: openai_compatible, model: m}
embeddings: {provider: gemini, model: e}
`)
	if got := cfg.Tiers[TierPremium].Routing; got == nil || !got.IsEmpty() {
		t.Errorf("tier routing = %+v, want the explicit opt-out, not the default", got)
	}
}

func TestResolve_LanesDoNotShareTheUpstreamPointer(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {only: [a]}}
tiers:
  premium: {provider: openai_compatible, model: m}
  cheap_cloud: {provider: openai_compatible, model: m}
embeddings: {provider: openai_compatible, model: e}
`)
	cfg.Tiers[TierPremium].Routing.Only[0] = "mutated"
	cfg.Tiers[TierPremium].Routing.Sort = SortPrice
	if got := cfg.Tiers[TierCheapCloud].Routing; got.Only[0] != "a" || got.Sort != "" {
		t.Errorf("tier cheap_cloud routing = %+v, changed with its sibling's", got)
	}
	if got := cfg.Embeddings.Routing.Only[0]; got != "a" {
		t.Errorf("embeddings only = %q, changed with a tier's", got)
	}
	if got := cfg.Providers[providerOpenAICompatible].Upstream.Only[0]; got != "a" {
		t.Errorf("provider only = %q, changed with a lane's", got)
	}
}

const twoProviders = `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api"}
tiers:
  premium: {provider: openai_compatible, model: m}
embeddings: {provider: gemini, model: e}
`

func TestDigest_HostChangeBumpsVersion(t *testing.T) {
	moved := strings.Replace(twoProviders, broker, "https://api.mistral.ai", 1)
	if versionOf(t, twoProviders) == versionOf(t, moved) {
		t.Error("re-pointing the provider's host kept the routing version; content its old model wrote would stay attributed to it")
	}
}

func TestDigest_UnboundProviderEntryDoesNotBumpVersionButBumpsRevision(t *testing.T) {
	staged := strings.Replace(twoProviders, "providers:\n", "providers:\n  ollama: {base_url: \"http://gpu-box:11434\"}\n", 1)
	before, after := mustParse(t, twoProviders), mustParse(t, staged)
	if before.RoutingVersion() != after.RoutingVersion() {
		t.Error("an entry no lane binds moved the routing version; it routes nothing, so no cached brief may regenerate")
	}
	if before.Revision() == after.Revision() {
		t.Error("an edit to an unbound entry kept the revision; a concurrent editor's write would overwrite it unseen")
	}
}

func TestValidateProviderEntries(t *testing.T) {
	for name, tc := range map[string]struct {
		profile   Profile
		providers map[string]ProviderSettings
		says      string // "" means accepted
	}{
		"an unknown provider": {ProfileCloudFrontier, map[string]ProviderSettings{"openrouter": {BaseURL: broker}}, "not a provider this build knows"},
		"upstream on a provider that fronts one host": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerVLLM: {Upstream: &OpenRouterRouting{Sort: SortThroughput}}}, "vllm serves one model from one host",
		},
		"upstream on a direct vendor host": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: "https://api.mistral.ai", Upstream: &OpenRouterRouting{}}}, "not an OpenRouter host",
		},
		"a reasoning effort in upstream": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: broker, Upstream: &OpenRouterRouting{ReasoningEffort: effortLow}}}, "set reasoning_effort on the tier",
		},
		"a misspelt upstream sort": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: broker, Upstream: &OpenRouterRouting{Sort: "fastest"}}}, "sort",
		},
		"a host that is not http(s)": {ProfileCloudFrontier, map[string]ProviderSettings{providerVLLM: {BaseURL: "ftp://x"}}, "not an http(s) URL"},
		"an unbound local host under sovereign": {
			ProfileSovereign, map[string]ProviderSettings{providerOllama: {BaseURL: "http://localhost:11434"}}, "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := RoutingConfig{Profile: tc.profile, Providers: tc.providers}
			err := cfg.validateProviderEntries()
			if tc.says == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("err = %v, want a refusal saying %q", err, tc.says)
			}
		})
	}
}

func TestBoundProviders_NamesEveryLaneInLiftOrder(t *testing.T) {
	cfg := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			TierPremium: {Provider: providerOpenAICompatible}, TierCheapCloud: {Provider: providerOpenAICompatible}, TierLocalSmall: {Provider: providerGemini},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible}},
		Decisions:  &DecisionsConfig{Provider: providerJevCompatible},
	}
	want := map[string][]string{
		providerOpenAICompatible: {"tier cheap_cloud", "tier premium", "the embeddings lane"},
		providerGemini:           {"tier local_small"},
		providerJevCompatible:    {"the decisions lane"},
	}
	if got := cfg.BoundProviders(); !reflect.DeepEqual(got, want) {
		t.Errorf("BoundProviders = %v, want %v", got, want)
	}
}

// A stored row written before canonical() existed carries resolved lanes, the
// product default included; loading it again must neither move the version nor
// pin that default onto the provider, where it would stop tracking the product.
func TestDigest_RefinalizingAResolvedConfigKeepsItsVersion(t *testing.T) {
	capped := strings.Replace(twoProviders, "model: m}", "model: m, reasoning_effort: low}", 1)
	resolved := mustParse(t, capped)
	reloaded, err := FromStored(resolved, nil)
	if err != nil {
		t.Fatalf("FromStored: %v", err)
	}
	if reloaded.RoutingVersion() != resolved.RoutingVersion() {
		t.Error("reloading a resolved config moved its routing version")
	}
	if got := reloaded.Providers[providerOpenAICompatible].Upstream; got != nil {
		t.Errorf("provider upstream = %+v after reload, want still absent", got)
	}
	if got := reloaded.Tiers[TierPremium].ReasoningEffort; got != effortLow {
		t.Errorf("tier effort = %q after reload, want %q", got, effortLow)
	}
}
