// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// requestTokens is what one step request costs by the ~4-bytes-per-token
// heuristic: the system prompt, the step schema the adapter sends beside it,
// and the transcript. Coarse, but the ceiling exists to stop runaway growth,
// not to bill by it.
func requestTokens(system string, schema json.RawMessage, msgs []model.Message) int {
	total := len(system) + len(schema)
	for _, m := range msgs {
		total += len(m.Content)
	}
	return total / 4
}

// StepCost is what every step of a run offered one set of tools pays before
// its transcript, counted the way window.bounded counts it.
type StepCost struct {
	// Tokens is the whole fixed cost: the frame, the listing and the step
	// schema. It is what the listing budget holds.
	Tokens int
	// Listing and Schema are its two per-tool parts. A tool's input schema is
	// in BOTH, once for the model to read and once for the provider to enforce.
	Listing int
	Schema  int
}

// FixedStepCost is the one measure of a tool set's per-step cost, shared by
// the runtime's own count and the composition's budget gate and page, so the
// published headroom is the headroom a run has.
//
// The language rule is excluded, as in SystemFrameTokens: it is the caller's,
// rendered per installation base language.
func FixedStepCost(specs []mcp.ToolSpec) StepCost {
	schema := stepSchema(specs)
	return StepCost{
		Tokens:  requestTokens(systemPrompt(specs, promptfence.New(), ""), schema, nil),
		Listing: len(ToolListing(AsOffered(specs))) / 4,
		Schema:  len(schema) / 4,
	}
}
