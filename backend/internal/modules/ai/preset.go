// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// presetShell is the deploy-config envelope a preset under config/presets/ is
// written in. Only the routing block is of interest, so the rest of the
// envelope is read and dropped rather than modelled.
type presetShell struct {
	Seeds struct {
		AIRouting yaml.Node `yaml:"ai_routing"`
	} `yaml:"seeds"`
}

// ParsePreset decodes a preset file's bytes the way an operator who pasted its
// `seeds.ai_routing` block into their own config would have it decoded.
//
// A preset is a whole deploy config on disk and a RoutingConfig once parsed, so
// something has to unwrap the envelope. Exported because two readers need the
// same answer — the gate that holds every preset parseable, and the
// certification page that reports what each preset binds — and a preset that
// unwrapped differently for one of them would let the page describe bindings
// the product would refuse.
func ParsePreset(raw []byte) (RoutingConfig, error) {
	var shell presetShell
	if err := yaml.Unmarshal(raw, &shell); err != nil {
		return RoutingConfig{}, fmt.Errorf("ai: preset: not a deploy config: %w", err)
	}
	if shell.Seeds.AIRouting.IsZero() {
		return RoutingConfig{}, fmt.Errorf("ai: preset: carries no seeds.ai_routing — a preset with no binding binds nothing")
	}
	inner, err := yaml.Marshal(&shell.Seeds.AIRouting)
	if err != nil {
		return RoutingConfig{}, fmt.Errorf("ai: preset: re-encoding the routing block: %w", err)
	}
	return ParseRouting(inner)
}
