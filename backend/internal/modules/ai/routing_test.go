// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestParseRoutingValidatesAtStartup(t *testing.T) {
	valid := `
tiers:
  local_small: {provider: fake}
  cheap_cloud: {provider: anthropic, model: claude-haiku}
embeddings: {provider: fake}
profile: eu_hosted
`
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"valid", valid, ""},
		{"missing profile", strings.Replace(valid, "profile: eu_hosted", "", 1), "profile is required"},
		{"unknown profile", strings.Replace(valid, "eu_hosted", "hybrid", 1), "unknown profile"},
		{"unknown tier", strings.Replace(valid, "local_small", "medium_cloud", 1), "unknown tier"},
		{"frontier binds", strings.Replace(valid, "cheap_cloud", "frontier", 1), ""},
		{"tier without provider", strings.Replace(valid, "{provider: fake}\n  cheap_cloud", "{model: gemma}\n  cheap_cloud", 1), "no provider"},
		{"no embeddings lane", strings.Replace(valid, "embeddings: {provider: fake}", "", 1), "embeddings lane has no provider"},
		{"typo'd key rejected", strings.Replace(valid, "tiers:", "tierz:", 1), "field tierz not found"},
		{"sovereign refuses cloud chat tier", strings.Replace(valid, "profile: eu_hosted", "profile: sovereign", 1), "sovereign forbids cloud provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRouting([]byte(tc.yaml))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid config rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestParseRoutingSetsDeterministicSourceHash pins the routing half of the
// spec §4 config-snapshot key: the same yaml bytes always produce the same
// digest (Router.installConfigSnapshot relies on this for the ON CONFLICT
// DO NOTHING dimension row to actually collapse), and a change to the
// bytes must change the digest — an operator swapping providers must
// produce a NEW config-snapshot row, not silently reuse the old one's hash.
func TestParseRoutingSetsDeterministicSourceHash(t *testing.T) {
	cfg := []byte("profile: eu_hosted\ntiers:\n  cheap_cloud: {provider: fake}\nembeddings: {provider: fake}\n")
	first, err := ParseRouting(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseRouting(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.sourceHash == "" {
		t.Fatal("sourceHash must be set on a successfully parsed config")
	}
	if first.sourceHash != second.sourceHash {
		t.Fatalf("identical bytes produced different hashes: %q vs %q", first.sourceHash, second.sourceHash)
	}
	changed := []byte("profile: eu_hosted\ntiers:\n  cheap_cloud: {provider: fake, model: other}\nembeddings: {provider: fake}\n")
	third, err := ParseRouting(changed)
	if err != nil {
		t.Fatal(err)
	}
	if third.sourceHash == first.sourceHash {
		t.Fatal("a changed routing config must produce a different sourceHash")
	}
}

// TestTaskLadderReportsTheRoutingTableAndNeverAliasesIt covers the aicert
// runner's dependency on TaskLadder: it must report exactly the routing
// table's rungs for a known task, empty for an unknown one (no panic on
// a bad key), and hand back a copy a caller can mutate freely without
// corrupting taskLadders for the next call.
func TestTaskLadderReportsTheRoutingTableAndNeverAliasesIt(t *testing.T) {
	got := TaskLadder(TaskSiteExtract)
	want := []Tier{TierPremium}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("TaskLadder(TaskSiteExtract) = %v, want %v", got, want)
	}
	got[0] = TierLocalSmall
	again := TaskLadder(TaskSiteExtract)
	if again[0] != TierPremium {
		t.Fatalf("mutating a returned ladder corrupted the package table: got %v on the next call", again)
	}
	if unknown := TaskLadder(Task("not_a_real_task")); unknown != nil {
		t.Fatalf("an unknown task should report a nil ladder, got %v", unknown)
	}
}

// TestProviderIsLocalMatchesTheUnexportedSet pins ProviderIsLocal (the
// aicert cert lane's cloud-only-latency-cap dependency) to exactly the
// same providers TestLocalOnlyMatchesLocalProvidersForEveryProvider
// already binds localProviders to — one invariant, one exported reader.
func TestProviderIsLocalMatchesTheUnexportedSet(t *testing.T) {
	local := []string{providerOllama, providerVLLM, ProviderFake}
	for _, p := range local {
		if !ProviderIsLocal(p) {
			t.Errorf("ProviderIsLocal(%q) = false, want true", p)
		}
	}
	cloud := []string{providerAnthropic, providerOpenAI, providerGemini, providerOpenAICompatible}
	for _, p := range cloud {
		if ProviderIsLocal(p) {
			t.Errorf("ProviderIsLocal(%q) = true, want false", p)
		}
	}
}

// A cloud provider on any tier or the embeddings lane is refused under the
// sovereign profile — zero egress by construction (spec §3.6).
func TestSovereignRefusesOpenAICompatible(t *testing.T) {
	cfg := []byte(`
profile: sovereign
tiers:
  cheap_cloud: {provider: openai_compatible, base_url: https://api.mistral.ai, model: m}
embeddings: {provider: ollama, model: bge-m3}
`)
	if _, err := ParseRouting(cfg); err == nil || !strings.Contains(err.Error(), "sovereign forbids cloud provider") {
		t.Fatalf("want sovereign-forbids-cloud, got %v", err)
	}
}

// The native cloud adapters are refused under sovereign too — the guarantee is
// bound to provider identity, not to any config flag (spec §3.6).
func TestSovereignRefusesNativeCloudProviders(t *testing.T) {
	for _, provider := range []string{"openai", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			cfg := []byte("profile: sovereign\ntiers:\n  premium: {provider: " + provider + ", model: m}\nembeddings: {provider: ollama, model: bge-m3}\n")
			if _, err := ParseRouting(cfg); err == nil || !strings.Contains(err.Error(), "sovereign forbids cloud provider") {
				t.Fatalf("%s: want sovereign-forbids-cloud, got %v", provider, err)
			}
		})
	}
}

// LocalOnly (the runtime capability) and localProviders (the parse-time set)
// are two encodings of "is this cloud"; they may never disagree.
func TestLocalOnlyMatchesLocalProvidersForEveryProvider(t *testing.T) {
	built := map[string]ProviderConfig{
		"fake":              {Provider: "fake"},
		"anthropic":         {Provider: "anthropic", Model: "m"},
		"ollama":            {Provider: "ollama", Model: "m"},
		"vllm":              {Provider: "vllm", Model: "m"},
		"openai_compatible": {Provider: "openai_compatible", BaseURL: "https://x", Model: "m"},
		"openai":            {Provider: "openai", Model: "m"},
		"gemini":            {Provider: "gemini", Model: "m"},
	}
	for _, name := range knownProviders {
		cfg, ok := built[name]
		if !ok {
			t.Fatalf("knownProviders has %q with no build recipe in this test — add one", name)
		}
		client, err := SelectBrain(cfg, allCloudKeys())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, want := client.Caps().LocalOnly, localProviders[name]; got != want {
			t.Fatalf("%s: Caps().LocalOnly=%v but localProviders=%v — encodings disagree", name, got, want)
		}
	}
}

// UnboundLadderWarnings is boot-loud, not boot-fatal: a task with no
// bound rung anywhere on its ladder gets one warning naming the task and
// the ladder it can't reach; a task with at least one bound rung
// (fallback or primary) is silent.
func TestUnboundLadderWarnings(t *testing.T) {
	// The property, not a transcript of the message.
	//
	// This used to be a hand-written list of every task and its ladder, which
	// went stale the moment a ladder changed and failed a new task for
	// existing. Deriving the expectation from taskLadders fixed the staleness
	// and bought nothing: re-implementing the same loop over the same map
	// produces a matching expectation for any behaviour at all. So the test
	// asks what the warning is FOR — a task with no bound rung is named, a task
	// with one is not — against tiers chosen so the two sets are non-empty and
	// different.
	bound := map[Tier]ProviderConfig{TierPremium: {Provider: "fake"}}
	got := RoutingConfig{Tiers: bound}.UnboundLadderWarnings()

	var wantNamed, wantSilent []Task
	for _, task := range AllTasks() {
		if slices.Contains(taskLadders[task], TierPremium) {
			wantSilent = append(wantSilent, task)
			continue
		}
		wantNamed = append(wantNamed, task)
	}
	if len(wantNamed) == 0 || len(wantSilent) == 0 {
		t.Fatalf("the chosen binding leaves %d tasks unbound and %d bound; the test proves nothing "+
			"unless both sets have members", len(wantNamed), len(wantSilent))
	}
	for _, task := range wantNamed {
		if !slices.ContainsFunc(got, func(w string) bool { return strings.Contains(w, "task "+string(task)+":") }) {
			t.Errorf("task %s has no bound rung and no warning names it; its calls will be refused silently", task)
		}
	}
	for _, task := range wantSilent {
		if slices.ContainsFunc(got, func(w string) bool { return strings.Contains(w, "task "+string(task)+":") }) {
			t.Errorf("task %s has a bound rung and was warned about anyway", task)
		}
	}
	// And a binding with nothing in it warns about everything, which is the
	// arm that catches a warning loop that silently stopped running.
	if none := (RoutingConfig{Tiers: map[Tier]ProviderConfig{}}).UnboundLadderWarnings(); len(none) != len(AllTasks()) {
		t.Errorf("an empty binding produced %d warnings for %d tasks, want one each", len(none), len(AllTasks()))
	}
}

func TestParseRoutingEmbedDimensions(t *testing.T) {
	const base = `
profile: eu_hosted
tiers:
  cheap_cloud: {provider: fake}
embeddings: {provider: fake, model: embed-model, dimensions: %d}
`
	for _, tc := range []struct {
		dims    int
		wantErr bool
	}{
		{0, false}, {-1, true}, {2001, true}, {1, false}, {768, false}, {2000, false},
	} {
		t.Run(fmt.Sprintf("dims=%d", tc.dims), func(t *testing.T) {
			got, err := ParseRouting([]byte(fmt.Sprintf(base, tc.dims)))
			if (err != nil) != tc.wantErr {
				t.Fatalf("dims=%d: err=%v wantErr=%v", tc.dims, err, tc.wantErr)
			}
			if err == nil {
				// An accepted width must be preserved verbatim (0 defaults
				// to 1536) — a parser silently rewriting 1/768/2000 to
				// another width would otherwise pass unnoticed.
				want := tc.dims
				if tc.dims == 0 {
					want = 1536
				}
				if got.Embeddings.Dimensions != want {
					t.Fatalf("dims=%d: parsed Dimensions=%d, want %d", tc.dims, got.Embeddings.Dimensions, want)
				}
			}
		})
	}
}

func TestParseRoutingSovereignAllLocalIsValid(t *testing.T) {
	cfg, err := ParseRouting([]byte(`
tiers:
  local_small: {provider: ollama, model: gemma3}
  local_large: {provider: ollama, model: llama3.3:70b}
embeddings: {provider: ollama, model: bge-m3}
profile: sovereign
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg = cfg.WithKeys(allCloudKeys())
	if cfg.Profile != ProfileSovereign || len(cfg.Tiers) != 2 {
		t.Fatalf("unexpected parse: %+v", cfg)
	}
}

// A binding this process cannot SERVE is refused at the write, not at the
// rebind.
//
// This is the failure it exists to stop, reported from a live install: an
// operator re-points every tier at a broker through Settings, the write is
// accepted, the screen says saved — and the running role then declines to adopt
// it, because SelectBrain will not build an openai_compatible client without a
// host. It keeps serving the binding it already had, deliberately, so the
// installation goes on answering with the OLD models while the stored document
// says otherwise. The only account of why is an error line in the server log.
//
// Refusing at the door turns that into a message on the form, next to the field
// that is missing.
func TestABindingWithNoHostIsRefusedRatherThanStoredUnservable(t *testing.T) {
	broker := ProviderConfig{Provider: "openai_compatible", Model: "mistralai/mistral-small-3.2-24b-instruct"}
	withHost := broker
	withHost.BaseURL = "https://openrouter.ai/api"

	for _, tc := range []struct {
		name    string
		cfg     RoutingConfig
		refused bool
		says    string
	}{
		{
			name: "a chat tier with no host",
			cfg: RoutingConfig{
				Profile:    ProfileEUHosted,
				Tiers:      map[Tier]ProviderConfig{TierCheapCloud: broker},
				Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: ProviderFake}},
			},
			refused: true, says: "no base_url",
		},
		{
			name: "the embeddings lane with no host",
			cfg: RoutingConfig{
				Profile:    ProfileEUHosted,
				Tiers:      map[Tier]ProviderConfig{TierCheapCloud: {Provider: ProviderFake}},
				Embeddings: EmbeddingsConfig{ProviderConfig: broker},
			},
			refused: true, says: "no base_url",
		},
		{
			// Under a sovereign profile the missing host is beside the point:
			// openai_compatible is refused there whatever its endpoint, and
			// answering about base_url first sends the reader to fill in a
			// field on a binding that is refused either way.
			name: "a sovereign profile outranks the missing host",
			cfg: RoutingConfig{
				Profile:    ProfileSovereign,
				Tiers:      map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerOllama, BaseURL: "http://127.0.0.1:11434"}},
				Embeddings: EmbeddingsConfig{ProviderConfig: broker},
			},
			refused: true, says: "forbids cloud provider",
		},
		{
			// The other arm, so the rule cannot pass by refusing everything.
			name: "the same binding with its host",
			cfg: RoutingConfig{
				Profile:    ProfileEUHosted,
				Tiers:      map[Tier]ProviderConfig{TierCheapCloud: withHost},
				Embeddings: EmbeddingsConfig{ProviderConfig: withHost},
			},
			refused: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.refused {
				if err == nil {
					t.Fatal("accepted a binding SelectBrain cannot build — it would store cleanly and never be adopted")
				}
				if !strings.Contains(err.Error(), tc.says) {
					t.Errorf("refusal reads %q, and must name what is missing", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused a servable binding: %v", err)
			}
			// And it really is servable: the refusal above must be about the
			// host rather than about the vendor being unwelcome.
			if _, err := SelectBrain(withHost, config.Static(map[string]string{"OPENAI_COMPATIBLE_API_KEY": "k"})); err != nil {
				t.Fatalf("validate accepted a binding SelectBrain then refused: %v", err)
			}
		})
	}
}
