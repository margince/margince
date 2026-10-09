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
		if p := binding.Routing.Provider; p.Sort != nil && p.Sort.By == ai.SortThroughput && p.RequireParameters != nil &&
			*p.RequireParameters {
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

// hostedLocalOnlyBindings ratifies each shipped configuration that binds a
// local-only task's every rung to a hosted provider — under which that task's
// prompt leaves the machine, which is what `local_only` says it must not do.
//
// Nothing refuses it. The tier a ladder names is a capability class the
// deployment binds, and `local_small` carries two jobs that pull apart.
// docs/explanation/ai-runtime.md says it runs "on the same box". And degradeTo
// makes it the floor every other rung falls to when a budget is spent. A
// cloud-only deployment has no local model and still needs a floor, so it
// binds the cheapest hosted one it has and the tier quietly stops meaning
// zero-egress.
//
// Which of those two jobs the tier keeps is an open product question, so this
// gate reports rather than enforces: the set cannot grow in silence, and the
// list below is the evidence that question needs.
var hostedLocalOnlyBindings = gatekit.Waive(map[string]string{
	"margince.dev.yaml": "the dev stack rides one vendor on every rung so a contributor needs no local " +
		"inference to boot it, and it judges seeded fixtures rather than a real mailbox",
	"presets/gemini_cloud.yaml": "an all-Gemini deployment has no local rung to offer",
	"presets/gemini_vertex_eu.yaml": "the same on Vertex AI, held to EU locations — which bounds the REGION " +
		"the prompt reaches but not the machine, and local_only is about the machine",
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
				"%s: task %s declares local_only and no rung of its ladder (%v) binds a local provider, so its prompt leaves the machine there",
				subject, task, ai.TaskLadder(task)))
		}
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("a shipped config sends a local-only task's prompt to a hosted provider without saying so — bind a local rung, or ratify it in hostedLocalOnlyBindings with the reason:\n  %s",
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
