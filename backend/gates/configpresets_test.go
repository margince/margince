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
// thing checking these files is the next person to paste one into production.
//
// The corpus is DERIVED from the directory rather than listed here, so a preset
// added later is covered by this gate without anybody remembering to add it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const presetDir = "../config/presets"

// presetShell is the deploy-config envelope a preset is written in; only the
// routing block is under test here.
type presetShell struct {
	Seeds struct {
		AIRouting yaml.Node `yaml:"ai_routing"`
	} `yaml:"seeds"`
}

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
	var shell presetShell
	if err := yaml.Unmarshal(raw, &shell); err != nil {
		t.Fatalf("%s: not a deploy config: %v", path, err)
	}
	if shell.Seeds.AIRouting.IsZero() {
		t.Fatalf("%s: carries no seeds.ai_routing — a preset with no binding binds nothing", path)
	}
	inner, err := yaml.Marshal(&shell.Seeds.AIRouting)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	cfg, err := ai.ParseRouting(inner)
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
