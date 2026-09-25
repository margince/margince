// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The routing form's one-click OpenRouter decision binding and the commented
// `decisions:` blocks the presets carry are two spellings of one binding: an
// operator who presses the button and one who uncomments the block must end up
// with the same endpoint and model. The form's constant is the source; every
// commented block naming its provider must agree with it, and every commented
// block, uncommented, must be a lane the server accepts under its preset's
// profile — a block kept in comments is exactly the one nothing else parses.

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

var (
	decisionPresetBlock = regexp.MustCompile(`(?s)export const OPENROUTER_DECISION_PRESET = \{(.*?)\} as const`)
	presetField         = regexp.MustCompile(`(?m)^\s*(provider|base_url|model): "([^"]+)",?$`)
	commentedLaneStart  = regexp.MustCompile(`^\s*# decisions:\s*$`)
	commentedLaneField  = regexp.MustCompile(`^\s*#   (provider|model|base_url): (\S+)\s*$`)
)

// readDecisionPreset returns the form's OpenRouter preset as a lane. A
// constant the pattern no longer finds fails rather than comparing nothing.
func readDecisionPreset(t *testing.T) ai.DecisionsConfig {
	t.Helper()
	raw, err := os.ReadFile(routingFieldsFile)
	if err != nil {
		t.Fatalf("read %s: %v", routingFieldsFile, err)
	}
	block := decisionPresetBlock.FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatalf("no `export const OPENROUTER_DECISION_PRESET = {...} as const` in %s — the mirror has gone blind", routingFieldsFile)
	}
	fields := map[string]string{}
	for _, m := range presetField.FindAllStringSubmatch(block[1], -1) {
		fields[m[1]] = m[2]
	}
	lane := ai.DecisionsConfig{Provider: fields["provider"], Model: fields["model"], BaseURL: fields["base_url"]}
	if lane.Provider == "" || lane.Model == "" || lane.BaseURL == "" {
		t.Fatalf("OPENROUTER_DECISION_PRESET parsed as %+v — a field the pattern missed would compare as empty", lane)
	}
	return lane
}

// commentedDecisionLanes reads every commented `# decisions:` block in a
// preset, each as the lane it would bind once uncommented.
func commentedDecisionLanes(src string) []ai.DecisionsConfig {
	var lanes []ai.DecisionsConfig
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if !commentedLaneStart.MatchString(line) {
			continue
		}
		var lane ai.DecisionsConfig
		for _, next := range lines[i+1:] {
			m := commentedLaneField.FindStringSubmatch(next)
			if m == nil {
				break
			}
			switch m[1] {
			case "provider":
				lane.Provider = m[2]
			case "model":
				lane.Model = m[2]
			case "base_url":
				lane.BaseURL = m[2]
			}
		}
		lanes = append(lanes, lane)
	}
	return lanes
}

func TestTheOpenRouterDecisionPresetIsTheCommentedPresetLane(t *testing.T) {
	t.Parallel()
	preset := readDecisionPreset(t)
	matched := 0
	for _, path := range presetFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		profile := routingFromPreset(t, path).Profile
		for _, lane := range commentedDecisionLanes(string(raw)) {
			if err := ai.ValidateDecisionsLane(profile, lane); err != nil {
				t.Errorf("%s: the commented decisions lane %+v, uncommented, is refused: %v", path, lane, err)
			}
			if lane.Provider != preset.Provider {
				continue
			}
			matched++
			if lane != preset {
				t.Errorf("%s: the commented decisions lane is %+v, the routing form's OpenRouter preset is %+v — an operator gets a different binding by pressing the button than by uncommenting the block", path, lane, preset)
			}
		}
	}
	if matched == 0 {
		t.Fatalf("no preset carries a commented %s decisions lane — this gate compared nothing", preset.Provider)
	}
}

// The preset names a model the price sheet carries, so the lane it binds is
// priced from its first call rather than read as unpriced.
func TestTheOpenRouterDecisionPresetIsPriced(t *testing.T) {
	t.Parallel()
	preset := readDecisionPreset(t)
	for _, rate := range ai.SeedModelRates(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)) {
		if rate.Provider == preset.Provider && rate.ModelID == preset.Model && rate.Lane == ai.LaneDecisions {
			return
		}
	}
	t.Errorf("no seeded decisions-lane rate for %s/%s, the model the routing form's OpenRouter preset binds", preset.Provider, preset.Model)
}

// The reader must see a block in each shape it claims, or the gate above
// reads green over presets whose blocks it could not parse.
func TestTheCommentedLaneReaderSeesEachBlock(t *testing.T) {
	t.Parallel()
	src := "    # decisions:\n    #   provider: jev_compatible\n    #   model: m\n    #   base_url: https://h/x\n" +
		"    # Or another:\n    # decisions:\n    #   provider: jev\n    #   model: jev-1.13.0\n    embeddings:\n"
	got := commentedDecisionLanes(src)
	want := []ai.DecisionsConfig{
		{Provider: "jev_compatible", Model: "m", BaseURL: "https://h/x"},
		{Provider: "jev", Model: "jev-1.13.0"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("commentedDecisionLanes = %+v, want %+v", got, want)
	}
}
