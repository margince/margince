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

	"gopkg.in/yaml.v3"
)

// qwenLocalVLLM is config/presets/qwen3_local_vllm.yaml's binding: chat tiers on
// vllm's compiled default and the embedder at a port of its own, because vLLM
// serves one model per process.
const qwenLocalVLLM = `profile: sovereign
tiers:
  local_small: {provider: vllm, model: "mlx-community/Qwen3-14B-4bit"}
  cheap_cloud: {provider: vllm, model: "mlx-community/Qwen3-14B-4bit"}
  premium: {provider: vllm, model: "mlx-community/Qwen3-14B-4bit"}
  frontier: {provider: vllm, model: "mlx-community/Qwen3-14B-4bit"}
embeddings: {provider: vllm, model: BAAI/bge-m3, base_url: "http://localhost:8001", dimensions: 1024}
`

// geminiBehindAGateway sends every chat tier through a proxy and embeds at the
// vendor directly, which an empty host meant before providers held hosts.
const geminiBehindAGateway = `profile: eu_hosted
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite, base_url: "https://eu-gateway.example"}
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite, base_url: "https://eu-gateway.example"}
  premium: {provider: gemini, model: gemini-3.5-flash, base_url: "https://eu-gateway.example"}
  frontier: {provider: gemini, model: gemini-3.1-pro-preview, base_url: "https://eu-gateway.example"}
embeddings: {provider: gemini, model: gemini-embedding-001, dimensions: 1536}
`

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
		"a broker binding whose tiers are served differently per model": {
			doc: `profile: cloud_frontier
tiers:
  local_small: {provider: openai_compatible, model: openai/gpt-oss-120b, base_url: "https://openrouter.ai/api"}
  cheap_cloud: {provider: openai_compatible, model: openai/gpt-oss-120b, base_url: "https://openrouter.ai/api"}
  premium: {provider: openai_compatible, model: mistralai/mistral-medium-3-5, base_url: "https://openrouter.ai/api", routing: {}}
  frontier: {provider: openai_compatible, model: anthropic/claude-sonnet-4.6, base_url: "https://openrouter.ai/api", routing: {quantizations: [fp16, bf16, fp32], require_parameters: true}}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: "https://openrouter.ai/api", dimensions: 1024}
`,
			want: "54b0da1f30ff47919042f00d9882f066910972f4dcdb0b55a2d408a912d76aeb",
		},
		"a local binding with a separate embeddings server": {
			doc:  qwenLocalVLLM,
			want: "8d8fff91f8bdf5171d385a521a601f7723ac92b14c1a2f8d0b3b6e1120e50260",
		},
		"native tiers behind a gateway and an embedder on the vendor": {
			doc:  geminiBehindAGateway,
			want: "2ee6e7d8d9ed2c3d827609956cfdd96b233ff3f476c383036bc6121d1f60de91",
		},
		"local tiers on a GPU box and an embedder on the compiled default": {
			doc: `profile: cloud_frontier
tiers:
  premium: {provider: vllm, model: m, base_url: "http://10.0.0.5:8000"}
embeddings: {provider: vllm, model: e}
`,
			want: "9b082bb3258f84115f7a16bb706305f17d0fc88c6019f24ec7b0de45431c5ff7",
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

func TestLift_ServingPrefsStayOnTheTier(t *testing.T) {
	log, _ := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium:    brokerLane("m", &OpenRouterRouting{Only: []string{"mistral/eu"}, Sort: SortThroughput, ReasoningEffort: effortHigh}),
		TierCheapCloud: brokerLane("m", &OpenRouterRouting{Only: []string{"mistral/eu"}}),
	}}
	lifted := cfg.liftLaneProviderFields(log)
	if got := lifted.Providers[providerOpenAICompatible].Upstream; !reflect.DeepEqual(got, &OpenRouterRouting{Only: []string{"mistral/eu"}}) {
		t.Errorf("provider upstream = %+v, want the pin alone", got)
	}
	if got := lifted.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, &OpenRouterRouting{Sort: SortThroughput, ReasoningEffort: effortHigh}) {
		t.Errorf("tier premium routing = %+v, want its sort and effort without the pin", got)
	}
	if got := lifted.Tiers[TierCheapCloud].Routing; got == nil || !got.IsEmpty() {
		t.Errorf("tier cheap_cloud routing = %+v, want `{}`: it never took the product default, and must not start", got)
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
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {only: [a], allow_fallbacks: false}}
tiers:
  premium: {provider: openai_compatible, model: m, routing: {sort: throughput, quantizations: [fp8]}}
embeddings: {provider: openai_compatible, model: e}
`)
	off := false
	if got, want := cfg.Embeddings.Routing, (&OpenRouterRouting{Only: []string{"a"}, AllowFallbacks: &off}); !reflect.DeepEqual(got, want) {
		t.Errorf("embeddings routing = %+v, want only the provider's pins %+v", got, want)
	}
	full := &OpenRouterRouting{Only: []string{"a"}, Sort: SortThroughput, Quantizations: []string{"fp8"}, AllowFallbacks: &off}
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, full) {
		t.Errorf("tier routing = %+v, want its own serving preferences plus the pins %+v", got, full)
	}
}

func TestResolve_AbsentTierRoutingGetsTheProductDefaultAndThePins(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {only: [mistral/eu]}}
tiers:
  premium: {provider: openai_compatible, model: m}
embeddings: {provider: gemini, model: e}
`)
	want := DefaultOpenRouterRouting()
	want.Only = []string{"mistral/eu"}
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, want) {
		t.Errorf("tier routing = %+v, want the product default pinned %+v", got, want)
	}
}

func TestResolve_TierOptOutKeepsOnlyProviderPins(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {only: [mistral/eu]}}
tiers:
  premium: {provider: openai_compatible, model: m, routing: {}}
embeddings: {provider: gemini, model: e}
`)
	if got, want := cfg.Tiers[TierPremium].Routing, (&OpenRouterRouting{Only: []string{"mistral/eu"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("tier routing = %+v, want the pin and none of the product default %+v", got, want)
	}
}

func TestResolve_AnEmptyProviderUpstreamPinsNothing(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
providers:
  openai_compatible: {base_url: "https://openrouter.ai/api", upstream: {}}
tiers:
  premium: {provider: openai_compatible, model: m}
  cheap_cloud: {provider: openai_compatible, model: m, routing: {}}
embeddings: {provider: openai_compatible, model: e}
`)
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, DefaultOpenRouterRouting()) {
		t.Errorf("tier premium routing = %+v, want the product default untouched", got)
	}
	if got := cfg.Tiers[TierCheapCloud].Routing; got == nil || !got.IsEmpty() {
		t.Errorf("tier cheap_cloud routing = %+v, want its opt-out kept", got)
	}
	if got := cfg.Embeddings.Routing; got != nil {
		t.Errorf("embeddings routing = %+v, want none", got)
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
	if got := cfg.Tiers[TierCheapCloud].Routing; got.Only[0] != "a" || got.Sort != SortThroughput {
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
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: broker, Upstream: &OpenRouterRouting{ReasoningEffort: effortLow}}}, "set per tier",
		},
		"a serving preference in upstream": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: broker, Upstream: &OpenRouterRouting{Sort: SortThroughput}}}, "set per tier",
		},
		"a pin naming one host twice": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: broker, Upstream: &OpenRouterRouting{Only: []string{"a", "a"}}}}, "twice",
		},
		"a proxy host on a vendor adapter": {
			ProfileCloudFrontier, map[string]ProviderSettings{providerAnthropic: {BaseURL: "https://gateway.example"}}, "",
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

// A stored row written before canonical() existed carries resolved lanes, pins
// and product default included; loading it again must neither move the version
// nor lift any serving preference onto the provider.
func TestDigest_RefinalizingAResolvedConfigKeepsItsVersion(t *testing.T) {
	pinned := strings.Replace(twoProviders, `{base_url: "https://openrouter.ai/api"}`, `{base_url: "https://openrouter.ai/api", upstream: {only: [mistral/eu]}}`, 1)
	resolved := mustParse(t, pinned)
	reloaded, err := FromStored(resolved, nil)
	if err != nil {
		t.Fatalf("FromStored: %v", err)
	}
	if reloaded.RoutingVersion() != resolved.RoutingVersion() {
		t.Error("reloading a resolved config moved its routing version")
	}
	if got := reloaded.Providers[providerOpenAICompatible].Upstream; !reflect.DeepEqual(got, &OpenRouterRouting{Only: []string{"mistral/eu"}}) {
		t.Errorf("provider upstream = %+v after reload, want the pin alone, with no serving preference pinned onto it", got)
	}
}

func TestResolve_ASeparateEmbeddingsServerKeepsItsHost(t *testing.T) {
	cfg := mustParse(t, qwenLocalVLLM)
	for tier, lane := range cfg.Tiers {
		if lane.BaseURL != "" {
			t.Errorf("tier %s resolved to %q, want vllm's compiled default", tier, lane.BaseURL)
		}
	}
	if got := cfg.Embeddings.BaseURL; got != "http://localhost:8001" {
		t.Errorf("embeddings resolved to %q, want its own server", got)
	}
	if got := cfg.canonical().Embeddings.BaseURL; got != "http://localhost:8001" {
		t.Errorf("canonical embeddings host = %q, want the override kept", got)
	}
}

func TestLift_ALaneAtTheDefaultAndALaneElsewhereDisagree(t *testing.T) {
	log, buf := warnings()
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: providerVLLM, Model: "m"},
		TierPremium:    {Provider: providerVLLM, Model: "m", BaseURL: "http://localhost:9000"},
	}}
	if got := cfg.liftLaneProviderFields(log).Providers[providerVLLM].BaseURL; got != "" {
		t.Errorf("provider host = %q, want the compiled default tier cheap_cloud dials", got)
	}
	lines := warnLines(buf)
	if len(lines) != 1 || !strings.Contains(lines[0], `"dropped":"tier premium"`) {
		t.Errorf("warnings = %v, want one dropping tier premium", lines)
	}
}

const pinnedEmbedder = `profile: cloud_frontier
tiers:
  premium: {provider: openai_compatible, model: anthropic/claude-sonnet-4.5, base_url: "https://openrouter.ai/api"}
embeddings: {provider: openai_compatible, model: mistralai/mistral-embed-2312, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu]}}
`

func TestLift_EmbeddingsPinsStayOnTheirLaneBesideATier(t *testing.T) {
	cfg := mustParse(t, pinnedEmbedder)
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, DefaultOpenRouterRouting()) {
		t.Errorf("tier routing = %+v, want the product default without the embedder's pin: the pinned host may not serve this model", got)
	}
	if got := cfg.Embeddings.Routing; !reflect.DeepEqual(got, &OpenRouterRouting{Only: []string{"mistral/eu"}}) {
		t.Errorf("embeddings routing = %+v, want its own pin", got)
	}
}

func TestLift_ALaneThatStatedNoPinsInheritsThemWithAWarning(t *testing.T) {
	const doc = `profile: cloud_frontier
tiers:
  cheap_cloud: {provider: openai_compatible, model: m, base_url: "https://openrouter.ai/api", routing: {only: [mistral/eu]}}
  premium: {provider: openai_compatible, model: m, base_url: "https://openrouter.ai/api"}
embeddings: {provider: openai_compatible, model: e, base_url: "https://openrouter.ai/api"}
`
	cfg := mustParse(t, doc)
	pinned := DefaultOpenRouterRouting()
	pinned.Only = []string{"mistral/eu"}
	if got := cfg.Tiers[TierPremium].Routing; !reflect.DeepEqual(got, pinned) {
		t.Errorf("tier premium routing = %+v, want the default under the provider's pin", got)
	}
	if got := cfg.Embeddings.Routing; !reflect.DeepEqual(got, &OpenRouterRouting{Only: []string{"mistral/eu"}}) {
		t.Errorf("embeddings routing = %+v, want the provider's pin", got)
	}
	var raw RoutingConfig
	if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
		t.Fatal(err)
	}
	log, buf := warnings()
	raw.liftLaneProviderFields(log)
	lines := warnLines(buf)
	if len(lines) != 2 || !strings.Contains(lines[0], `"inheriting":"tier premium"`) || !strings.Contains(lines[1], `"inheriting":"the embeddings lane"`) ||
		!strings.Contains(lines[0], `"pinned_by":"tier cheap_cloud"`) {
		t.Errorf("warnings = %v, want one per inheriting lane naming tier cheap_cloud", lines)
	}
}

func TestLift_PinsOnADirectVendorHostAreDroppedNotRefused(t *testing.T) {
	cfg := mustParse(t, `profile: cloud_frontier
tiers:
  premium: {provider: openai_compatible, model: m, base_url: "https://api.mistral.ai", routing: {only: [mistral/eu]}}
embeddings: {provider: gemini, model: e}
`)
	if got := cfg.Tiers[TierPremium].Routing; got != nil {
		t.Errorf("tier routing = %+v, want none: a direct vendor takes no broker pins", got)
	}
	if cfg.Providers[providerOpenAICompatible].Upstream != nil {
		t.Error("the dropped pins reached the provider")
	}
}

func TestResolve_ADirectVendorTierBesideAPinnedBrokerEmbedder(t *testing.T) {
	doc := strings.Replace(pinnedEmbedder, `model: anthropic/claude-sonnet-4.5, base_url: "https://openrouter.ai/api"`, `model: mistral-medium, base_url: "https://api.mistral.ai"`, 1)
	cfg := mustParse(t, doc)
	if got := cfg.Tiers[TierPremium]; got.BaseURL != "https://api.mistral.ai" || got.Routing != nil {
		t.Errorf("tier = %+v, want the vendor host with no broker preferences", got)
	}
	if got := cfg.Embeddings; got.BaseURL != broker || !reflect.DeepEqual(got.Routing, &OpenRouterRouting{Only: []string{"mistral/eu"}}) {
		t.Errorf("embeddings = %+v, want the broker and its own pin", got.ProviderConfig)
	}
}

func TestRevision_SeesTheHostOfAnUnliftedRow(t *testing.T) {
	row := func(host string) RoutingConfig {
		return RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{TierPremium: {Provider: providerOpenAICompatible, Model: "m", BaseURL: host}}}
	}
	if row("https://a.example").Revision() == row("https://b.example").Revision() {
		t.Error("two stored rows differing only in a tier's host share a revision; a concurrent host edit would be overwritten unseen")
	}
}

func TestLift_AnEmbedderOnTheVendorStaysOffItsTiersGateway(t *testing.T) {
	var raw RoutingConfig
	if err := yaml.Unmarshal([]byte(geminiBehindAGateway), &raw); err != nil {
		t.Fatal(err)
	}
	log, buf := warnings()
	raw.liftLaneProviderFields(log)
	if lines := warnLines(buf); len(lines) != 0 {
		t.Errorf("warnings = %v, want none: nothing dials anywhere new", lines)
	}
	cfg := mustParse(t, geminiBehindAGateway)
	if got := cfg.Tiers[TierPremium].BaseURL; got != "https://eu-gateway.example" {
		t.Errorf("tier premium resolved to %q, want the gateway", got)
	}
	if got := cfg.Embeddings.BaseURL; !sameHost(providerGemini, got, "") {
		t.Errorf("embeddings resolved to %q, want the Gemini API it dialled before", got)
	}
}

// Writing a provider's compiled default out is how a lift keeps a lane where
// it was, which is only true if the adapter dials it exactly as it dials none.
func TestEveryCompiledHostDialsAsAnEmptyOneDoes(t *testing.T) {
	for _, provider := range providerNamesWhere(speaksChat) {
		host := compiledHost(provider)
		if host == "" {
			continue
		}
		model := ProviderConfig{Provider: provider, Model: "m"}
		spelled := model
		spelled.BaseURL = host
		if got, want := clientBaseURL(t, spelled), clientBaseURL(t, model); got != want {
			t.Errorf("%s: base_url %q dials %q, an empty one dials %q", provider, host, got, want)
		}
	}
}

func clientBaseURL(t *testing.T, binding ProviderConfig) string {
	t.Helper()
	client, err := SelectBrain(binding, allCloudKeys(t))
	if err != nil {
		t.Fatalf("SelectBrain(%s): %v", binding.Provider, err)
	}
	switch c := client.(type) {
	case *anthropicClient:
		return c.baseURL
	case *openaiClient:
		return c.baseURL
	case *ollamaClient:
		return c.baseURL
	case *openAICompatClient:
		return c.baseURL
	case *geminiClient:
		return c.transport.modelsURL()
	}
	t.Fatalf("%s: %T has a compiled host this test cannot read; add its case", binding.Provider, client)
	return ""
}
