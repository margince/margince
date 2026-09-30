// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The two ways a file can fail to be a preset, and the alias case that is the
// reason this unwraps through deployconfig rather than marshalling the subtree
// itself.

import (
	"strings"
	"testing"
)

func TestParsePresetRefusesWhatIsNotAPreset(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ raw, wants string }{
		"not yaml at all": {
			raw:   "\tthis: [is not\n  yaml",
			wants: "not a deploy config",
		},
		// A deploy config is not a preset until it declares a binding, and the
		// refusal has to say which of the two it is: an operator who copied the
		// wrong half of a file gets told so.
		"a deploy config declaring no binding": {
			raw:   "version: 1\nseeds:\n  starter_automations: true\n",
			wants: "carries no seeds.ai_routing",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParsePreset([]byte(tc.raw))
			if err == nil {
				t.Fatal("parsed something that is not a preset")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("error %q does not say %q", err, tc.wants)
			}
		})
	}
}

// An anchor defined outside the routing block and referenced inside it is
// ordinary YAML, and marshalling the subtree alone would emit the alias with no
// anchor in scope — "unknown anchor 'd' referenced", a parser detail handed to
// an operator whose file is valid. This is what ParsePreset shares with
// bootstrap rather than re-spelling.
func TestParsePresetResolvesAnAnchorDefinedOutsideTheBlock(t *testing.T) {
	t.Parallel()
	const raw = `
version: 1
defaults: &brokered
  provider: openai_compatible
  base_url: https://openrouter.ai/api
seeds:
  ai_routing:
    profile: cloud_frontier
    tiers:
      cheap_cloud:
        <<: *brokered
        model: mistralai/ministral-8b-2512
    embeddings:
      <<: *brokered
      model: mistralai/mistral-embed-2312
`
	cfg, err := ParsePreset([]byte(raw))
	if err != nil {
		t.Fatalf("a preset using an anchor its own document defines must parse: %v", err)
	}
	if got := cfg.Tiers[TierCheapCloud].Model; got != "mistralai/ministral-8b-2512" {
		t.Errorf("cheap_cloud model = %q", got)
	}
	if got := cfg.Tiers[TierCheapCloud].BaseURL; got == "" {
		t.Error("the merged-in base_url did not survive the unwrap")
	}
}
