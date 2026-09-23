// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// ParsePreset decodes a preset file's bytes the way bootstrap decodes the same
// block out of a deployment's own config: through deployconfig's unwrapper,
// which resolves YAML aliases before re-encoding the subtree, and then through
// this package's routing parser.
//
// Both steps are shared rather than re-spelled. A preset exists to be copied,
// so a reader that unwrapped differently from the boot path could call a file
// unparseable that an operator's deployment starts on — and a preset the gate
// refuses while production accepts it is worse than no preset.
func ParsePreset(raw []byte) (RoutingConfig, error) {
	var shell struct {
		Seeds struct {
			AIRouting yaml.Node `yaml:"ai_routing"`
		} `yaml:"seeds"`
	}
	if err := yaml.Unmarshal(raw, &shell); err != nil {
		return RoutingConfig{}, fmt.Errorf("ai: preset: not a deploy config: %w", err)
	}
	if shell.Seeds.AIRouting.IsZero() {
		return RoutingConfig{}, fmt.Errorf("ai: preset: carries no seeds.ai_routing — a preset with no binding binds nothing")
	}
	inner, err := deployconfig.SeedSubtreeBytes(shell.Seeds.AIRouting)
	if err != nil {
		return RoutingConfig{}, fmt.Errorf("ai: preset: %w", err)
	}
	return ParseRouting(inner)
}
