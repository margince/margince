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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/gatekit"
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

// residencyGaps names every lane of cfg whose text could be read outside the EU:
// a lane on a binding that cannot carry a host pin, and a broker lane whose
// `only:` is absent or admits a host that is not an EU-region endpoint. An empty
// `routing: {}` is the broker's own price-weighted choice of host, anywhere.
//
// The broker half is ai.EURegionPinGap, the rule the parser holds every
// eu_hosted config to. The first half is this gate's own and is stricter: the
// parser admits a native vendor under eu_hosted because it cannot tell where an
// operator's host runs, but a SHIPPED preset is one the repository vouches for,
// and a binding that carries no pin is not one it can vouch for.
func residencyGaps(cfg ai.RoutingConfig) []string {
	lanes := map[string]ai.ProviderConfig{"embeddings": cfg.Embeddings.ProviderConfig}
	for tier, binding := range cfg.Tiers {
		lanes[string(tier)] = binding
	}
	var gaps []string
	for lane, binding := range lanes {
		if !ai.UpstreamPreferencesApply(binding) {
			host := binding.BaseURL
			if host == "" {
				host = "its vendor's own host"
			}
			gaps = append(gaps, lane+": bound to "+binding.Provider+" at "+host+", which carries no host pin")
			continue
		}
		if gap := ai.EURegionPinGap(binding); gap != "" {
			gaps = append(gaps, lane+": "+gap)
		}
	}
	sort.Strings(gaps)
	return gaps
}

// A preset that declares eu_hosted pins every lane to an EU-region endpoint.
//
// Selected by the profile the preset DECLARES, not by its file name: the
// profile is what an operator copies into their deployment and what every
// certification record from it is filed under, so it is the promise. A broker
// serves one model id from several regions and, without a pin, picks among
// them itself, so the pin is the whole residency guarantee. The embeddings lane
// is held too, because it reads every document the chat tiers do.
//
// Held by: TestAResidencyPresetPinsEveryLaneToAnEURegion (backend/gates/configpresets_test.go) — this test.
func TestAResidencyPresetPinsEveryLaneToAnEURegion(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, path := range presetFiles(t) {
		cfg := routingFromPreset(t, path)
		if cfg.Profile != ai.ProfileEUHosted {
			continue
		}
		checked++
		for _, gap := range residencyGaps(cfg) {
			t.Errorf("%s: %s", filepath.Base(path), gap)
		}
	}
	if checked == 0 {
		t.Fatalf("no preset in %s declares %s, so this gate is vouching for nothing", presetDir, ai.ProfileEUHosted)
	}
}

// Every shape of an unpinned lane is caught, each by the finding that names it.
//
// The planted configs declare cloud_frontier because the parser refuses an
// unpinned broker lane under eu_hosted before this gate could see it;
// residencyGaps reads the lanes and never the profile, so the planted shapes
// are the same ones.
func TestResidencyGapsSeesEveryUnpinnedShape(t *testing.T) {
	t.Parallel()
	const pinned = "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}"
	const embeddings = "embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}\n"
	withPremium := func(premium string) string {
		return "profile: cloud_frontier\ntiers:\n  cheap_cloud: " + pinned + "\n  premium: " + premium + "\n" + embeddings
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
			"profile: cloud_frontier\ntiers:\n  premium: " + pinned + "\n" +
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

// hostedLocalOnlyBindings ratifies each shipped configuration that binds a
// local-only task's every rung to a hosted provider — under which that task
// does not run at all, because the router drops the rung.
//
// These are not oversights and this is not a backlog. `local_small` is a SIZE
// class in this tree, not a location: every file below binds it deliberately,
// and the two capture verdicts are the only tasks that ask for more than size.
// What the waiver buys is that the set cannot grow in silence — a new preset,
// or a rebind of an existing one, fails this gate until somebody writes down
// that they know the sender and thread verdicts will not run under it.
var hostedLocalOnlyBindings = gatekit.Waive(map[string]string{
	"margince.dev.yaml": "the dev stack rides one vendor on every rung so a contributor needs no local " +
		"inference to boot it, and it judges seeded fixtures rather than a real mailbox",
	"presets/gemini_cloud.yaml":     "an all-Gemini deployment has no local rung to offer",
	"presets/openrouter_cloud.yaml": "a broker deployment has no local rung to offer",
	"presets/openrouter_cloud_eu.yaml": "the same, pinned to EU endpoints — which bounds the REGION the " +
		"prompt reaches but not the machine, and local_only is about the machine",
	"presets/consumer_class_brokered.yaml": "it exists to measure consumer-class WEIGHTS through a broker " +
		"before the Ollama binding of those same weights exists; its own header says so, and names the " +
		"endpoints it actually reaches",
})

// A shipped config that silently excludes a local-only task says so here.
//
// capture_counterparty_verdict and capture_confidentiality_verdict declare
// `local_only` in api/ai-tasks.yaml because their prompts carry the subject and
// body of mail nobody has judged yet. The router enforces that at the rung: a
// hosted binding is dropped rather than called. That is the guarantee working,
// and it is also invisible — an operator reads a shipped task list and has no
// way to learn that their preset excludes two of them. Pinning the set is what
// turns the exclusion into a written decision instead of a surprise.
func TestEveryShippedConfigIsHonestAboutWhereALocalOnlyTaskRuns(t *testing.T) {
	t.Parallel()
	local := ai.LocalOnlyTasks()
	if len(local) == 0 {
		t.Fatal("no task declares local_only — this gate is watching nothing")
	}
	paths := shippedRoutingFiles(t)
	checked, bindsNothing := 0, 0
	var findings []string
	for _, path := range paths {
		cfg, bound := shippedRouting(t, path)
		if !bound {
			// A template with its routing block commented out binds nothing and
			// owes nothing. COUNTED, not skipped — see the tally below.
			bindsNothing++
			continue
		}
		checked++
		for _, task := range local {
			if servesLocally(cfg, task) {
				continue
			}
			subject := configSubject(path)
			if hostedLocalOnlyBindings.Waived(t, subject) {
				continue
			}
			findings = append(findings, fmt.Sprintf(
				"%s: task %s is local_only and no rung of its ladder (%v) binds a local provider, so it will not run there",
				subject, task, ai.TaskLadder(task)))
		}
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("a shipped config excludes a local-only task without saying so — bind a local rung, or ratify it in hostedLocalOnlyBindings with the reason:\n  %s",
			strings.Join(findings, "\n  "))
	}
	// The one way this gate can go quiet is by reading a smaller corpus: a file
	// it never reached reports exactly like a clean one. Every path is either
	// checked or counted as binding nothing, and nothing falls between.
	if checked+bindsNothing != len(paths) {
		t.Errorf("%d of %d shipped configs were neither checked nor counted as binding nothing",
			len(paths)-checked-bindsNothing, len(paths))
	}
	if checked == 0 {
		t.Fatal("no shipped config bound anything — the corpus parsed to nothing and this gate checked nothing")
	}
	hostedLocalOnlyBindings.AssertAllMatched(t)
}

// servesLocally reports whether cfg binds at least one rung of task's ladder to
// a local provider — the question the router asks per call, over a parsed
// config rather than an installed binding.
func servesLocally(cfg ai.RoutingConfig, task ai.Task) bool {
	for _, tier := range ai.TaskLadder(task) {
		if binding, bound := cfg.Tiers[tier]; bound && ai.ProviderIsLocal(binding.Provider) {
			return true
		}
	}
	return false
}

// shippedRoutingFiles collects the configurations an operator boots on or
// copies from: the presets, plus config/margince*.yaml — the one a contributor
// actually runs, and so the one whose exclusions bite first.
//
// Both halves are DERIVED from their directory rather than listed, so a config
// added later arrives inside this gate rather than beside it.
func shippedRoutingFiles(t *testing.T) []string {
	t.Helper()
	paths := presetFiles(t)
	deployed, err := filepath.Glob("../config/margince*.yaml")
	if err != nil {
		t.Fatalf("globbing the shipped configs: %v", err)
	}
	// NOT a tolerated zero: the tree ships these, so an empty glob means the
	// path moved and this gate went half-blind while still reporting PASS.
	if len(deployed) == 0 {
		t.Fatal("no config/margince*.yaml found — the corpus moved")
	}
	return append(paths, deployed...)
}

// shippedRouting parses one shipped config, separating "binds nothing" from
// "binds something broken". routingFromPreset cannot: it fatals on both, and
// config/margince.example.yaml is a template whose routing block is entirely
// commented out.
func shippedRouting(t *testing.T, path string) (ai.RoutingConfig, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	cfg, err := ai.ParsePreset(raw)
	if errors.Is(err, ai.ErrNoRoutingBlock) {
		return ai.RoutingConfig{}, false
	}
	if err != nil {
		t.Fatalf("%s declares a routing block that does not parse: %v", configSubject(path), err)
	}
	return cfg, true
}

// configSubject names a file the way the waiver map does — relative to config/,
// so an entry reads as the path an operator would open.
func configSubject(path string) string {
	dir, file := filepath.Split(filepath.ToSlash(path))
	if filepath.Base(filepath.Clean(dir)) == "presets" {
		return "presets/" + file
	}
	return file
}
