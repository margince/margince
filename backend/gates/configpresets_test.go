// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every preset under config/presets/ is a binding the parser accepts.
//
// A preset exists to be copied by an operator, so a preset the parser refuses is
// worse than no preset: it is a file in the repository telling somebody to write
// something that will not boot. Nothing loads this directory at runtime — that
// is deliberate, and it is exactly why a test has to, because otherwise the only
// thing checking these files is the next contact to paste one into production.
//
// The corpus is DERIVED from the directory rather than listed here, so a preset
// added later is covered by this gate without anybody remembering to add it.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const presetDir = "../config/presets"

func presetFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(presetDir)
	if err != nil {
		t.Fatalf("reading %s: %v", presetDir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			files = append(files, filepath.Join(presetDir, e.Name()))
		}
	}
	if len(files) == 0 {
		t.Fatalf("no presets found in %s — this gate would pass by reading an empty tree", presetDir)
	}
	return files
}

func routingFromPreset(t *testing.T, path string) ai.RoutingConfig {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	// ai.ParsePreset and not a local unwrap: the certification page reports
	// what each preset binds by parsing the same files, and two unwrappers
	// would let that page describe a binding this gate never checked.
	cfg, err := ai.ParsePreset(raw)
	if err != nil {
		t.Fatalf("%s: the parser refuses this preset, so an operator who copied it could not boot: %v", path, err)
	}
	return cfg
}

// Held by: TestEveryConfigPresetParses (backend/gates/configpresets_test.go) — this test.
func TestEveryConfigPresetParses(t *testing.T) {
	t.Parallel()
	for _, path := range presetFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg := routingFromPreset(t, path)
			if len(cfg.Tiers) == 0 {
				t.Error("no tiers bound")
			}
			if cfg.Embeddings.Provider == "" {
				t.Error("no embeddings lane bound")
			}
		})
	}
}

// A broker tier that writes no preferences comes out of the parser carrying the
// product default, and a tier that writes an empty block comes out carrying
// none.
//
// This is the distinction the whole three-state design rests on, and it is the
// one a reader is most likely to assume away: an omitted key and an explicit
// empty object look alike in YAML and mean opposite things here. The openrouter
// preset is written to exercise both, so if that ever stops being true this
// asserts it rather than passing vacuously.
func TestABrokerPresetInheritsTheDefaultAndCanOptOut(t *testing.T) {
	t.Parallel()
	const path = presetDir + "/openrouter_cloud.yaml"
	cfg := routingFromPreset(t, path)

	var inherited, optedOut int
	for tier, binding := range cfg.Tiers {
		if !ai.IsOpenRouterHost(binding.BaseURL) {
			continue
		}
		if binding.Routing == nil {
			t.Errorf("tier %s: a broker binding must never reach the router with nil preferences — "+
				"either the default applied or the operator opted out, and nil is neither", tier)
			continue
		}
		if binding.Routing.IsEmpty() {
			optedOut++
			continue
		}
		if binding.Routing.Sort == ai.SortThroughput && binding.Routing.RequireParameters != nil &&
			*binding.Routing.RequireParameters {
			inherited++
		}
	}
	if inherited == 0 {
		t.Error("no tier inherited the reliability-over-price default; this preset no longer demonstrates it")
	}
	if optedOut == 0 {
		t.Error("no tier opts out with an empty block; the opt-out path is now untested by this preset")
	}
}

// residencyPresetSuffix marks a preset that promises where its text is read:
// openrouter_cloud_eu.yaml today. The name is the promise an operator reads when
// choosing one, so the name is what this gate holds to it.
const residencyPresetSuffix = "_eu.yaml"

// euRegionSlug reports whether a broker host slug names one EU-region endpoint.
// A base slug (`mistral`) matches every region the vendor serves from, and a
// variant slug (`mistral/zdr`) names a retention policy rather than a place, so
// neither pins anything to the EU.
func euRegionSlug(slug string) bool {
	_, region, found := strings.Cut(slug, "/")
	if !found {
		return false
	}
	return region == "eu" || strings.HasPrefix(region, "eu-") || strings.HasPrefix(region, "europe-")
}

// residencyGaps names every lane of cfg whose text could be read outside the EU:
// a lane on a binding that cannot carry a host pin, a lane with no `only:`, and
// a lane whose `only:` admits a host that is not an EU-region endpoint. An empty
// `routing: {}` is the broker's own price-weighted choice of host, anywhere.
func residencyGaps(cfg ai.RoutingConfig) []string {
	lanes := map[string]ai.ProviderConfig{"embeddings": cfg.Embeddings.ProviderConfig}
	for tier, binding := range cfg.Tiers {
		lanes[string(tier)] = binding
	}
	var gaps []string
	for lane, binding := range lanes {
		switch {
		case !ai.UpstreamPreferencesApply(binding):
			gaps = append(gaps, lane+": bound to "+binding.Provider+" at "+binding.BaseURL+", which carries no host pin")
		case binding.Routing == nil || len(binding.Routing.Only) == 0:
			gaps = append(gaps, lane+": no `only:` — the broker may serve "+binding.Model+" from any region")
		default:
			for _, slug := range binding.Routing.Only {
				if !euRegionSlug(slug) {
					gaps = append(gaps, lane+": `only:` admits "+slug+", which is not an EU-region endpoint")
				}
			}
		}
	}
	sort.Strings(gaps)
	return gaps
}

// A preset named for the EU pins every lane to an EU-region endpoint.
//
// OpenRouter serves one model id from several regions, and without a pin it
// picks among them itself — a model with no EU endpoint at all is still served,
// from wherever it runs. So the pin is the whole residency guarantee: a tier
// that inherits the product default, or opts out with `routing: {}`, sends its
// text out of region with nothing failing. The embeddings lane is held too,
// because it reads every document the chat tiers do.
//
// Held by: TestAResidencyPresetPinsEveryLaneToAnEURegion (backend/gates/configpresets_test.go) — this test.
func TestAResidencyPresetPinsEveryLaneToAnEURegion(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, path := range presetFiles(t) {
		if !strings.HasSuffix(path, residencyPresetSuffix) {
			continue
		}
		checked++
		for _, gap := range residencyGaps(routingFromPreset(t, path)) {
			t.Errorf("%s: %s", filepath.Base(path), gap)
		}
	}
	if checked == 0 {
		t.Fatalf("no preset in %s ends in %s, so this gate is vouching for nothing", presetDir, residencyPresetSuffix)
	}
}

// Every shape of an unpinned lane is caught, each by the finding that names it.
func TestResidencyGapsSeesEveryUnpinnedShape(t *testing.T) {
	t.Parallel()
	const pinned = "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}"
	const embeddings = "embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}\n"
	withPremium := func(premium string) string {
		return "profile: eu_hosted\ntiers:\n  cheap_cloud: " + pinned + "\n  premium: " + premium + "\n" + embeddings
	}
	for name, tc := range map[string]struct {
		yaml string
		want string
	}{
		"an inherited default": {
			withPremium("{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api'}"), "premium: no `only:`",
		},
		"an explicit opt-out": {
			withPremium("{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {}}"), "premium: no `only:`",
		},
		"a base slug": {
			withPremium("{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral]}}"),
			"premium: `only:` admits mistral,",
		},
		"a policy variant": {
			withPremium("{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu, mistral/zdr]}}"),
			"premium: `only:` admits mistral/zdr,",
		},
		"a direct vendor": {withPremium("{provider: gemini, model: m}"), "premium: bound to gemini"},
		"an unpinned embeddings lane": {
			"profile: eu_hosted\ntiers:\n  premium: " + pinned + "\n" +
				"embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api'}\n",
			"embeddings: no `only:`",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := ai.ParseRouting([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("the planted config does not parse, so it proves nothing: %v", err)
			}
			gaps := residencyGaps(cfg)
			if len(gaps) != 1 || !strings.HasPrefix(gaps[0], tc.want) {
				t.Errorf("gaps = %q, want exactly one starting %q", gaps, tc.want)
			}
		})
	}
	cfg, err := ai.ParseRouting([]byte(withPremium(pinned)))
	if err != nil {
		t.Fatalf("the control does not parse: %v", err)
	}
	if gaps := residencyGaps(cfg); len(gaps) != 0 {
		t.Errorf("a config pinned on every lane reports %q", gaps)
	}
}
