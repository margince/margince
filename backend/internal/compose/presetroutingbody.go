// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// PresetRoutingBody is a preset file's seeds.ai_routing as the body
// PUT /v1/ai/routing takes: parsed by the product's own preset reader and
// mapped to the wire by the same function the GET answers with, so binding a
// stack to a committed preset goes through the endpoint an operator uses.
func PresetRoutingBody(raw []byte) ([]byte, error) {
	cfg, err := ai.ParsePreset(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(toContractAiRouting(cfg))
}
