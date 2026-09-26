// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A thinking floor on the Anthropic wire. Anthropic's per-model table decides
// the shape: 4.6-4.8 take `thinking: {type: "adaptive"}` and think only when
// asked; 4.5 and earlier take only `{type: "enabled", budget_tokens: N}`; 5.x
// think by default at effort high, which meets every floor. `output_config.effort`
// is never sent: it shapes the whole answer, not just the thinking, and its
// default (high) is already the deepest a floor names.

import "strings"

type anthropicThinkingMode int

const (
	anthropicThinksUnknown anthropicThinkingMode = iota
	// anthropicThinksByDefault is a model that thinks unasked, so no floor
	// needs sending.
	anthropicThinksByDefault
	anthropicThinksAdaptive
	anthropicThinksWithBudget
)

// anthropicModelPrefixes maps a model id prefix to how it thinks, from
// Anthropic's "thinking support, defaults, and rejected configurations" table.
// An id matching none is unknown and sent nothing: a wrong shape is a 400.
var anthropicModelPrefixes = []struct {
	prefix string
	mode   anthropicThinkingMode
}{
	{"claude-opus-4-6", anthropicThinksAdaptive},
	{"claude-sonnet-4-6", anthropicThinksAdaptive},
	{"claude-opus-4-7", anthropicThinksAdaptive},
	{"claude-opus-4-8", anthropicThinksAdaptive},
	{"claude-opus-4-5", anthropicThinksWithBudget},
	{"claude-sonnet-4-5", anthropicThinksWithBudget},
	{"claude-haiku-4-5", anthropicThinksWithBudget},
	{"claude-opus-4-1", anthropicThinksWithBudget},
	{"claude-opus-4-0", anthropicThinksWithBudget},
	{"claude-opus-4-2025", anthropicThinksWithBudget},
	{"claude-sonnet-4-0", anthropicThinksWithBudget},
	{"claude-sonnet-4-2025", anthropicThinksWithBudget},
	{"claude-3-7-sonnet", anthropicThinksWithBudget},
	{"claude-opus-5", anthropicThinksByDefault},
	{"claude-sonnet-5", anthropicThinksByDefault},
	{"claude-fable-5", anthropicThinksByDefault},
	{"claude-mythos-5", anthropicThinksByDefault},
}

func anthropicThinkingModeOf(modelID string) anthropicThinkingMode {
	for _, known := range anthropicModelPrefixes {
		if strings.HasPrefix(modelID, known.prefix) {
			return known.mode
		}
	}
	return anthropicThinksUnknown
}

// anthropicThinkingBudgets is the budget_tokens a floor asks for. 1,024 is the
// API's minimum, so minimal and low share it.
var anthropicThinkingBudgets = map[string]int{effortMinimal: 1024, effortLow: 1024, effortMedium: 4096, effortHigh: 16384}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// anthropicThinkingFor is the `thinking` block a request is sent, nil for none.
//
// A request carrying tools is sent none: a thinking turn that calls a tool must
// be replayed with its thinking block, and this adapter keeps text only. A
// budget that would not leave room for the answer (it must be below
// max_tokens) is not sent either: the floor cannot be met without a cut answer.
func anthropicThinkingFor(modelID, floor string, maxTokens, tools int) *anthropicThinking {
	if floor == "" || tools > 0 {
		return nil
	}
	switch anthropicThinkingModeOf(modelID) {
	case anthropicThinksAdaptive:
		return &anthropicThinking{Type: "adaptive"}
	case anthropicThinksWithBudget:
		budget, ok := anthropicThinkingBudgets[floor]
		if !ok || budget >= maxTokens {
			return nil
		}
		return &anthropicThinking{Type: "enabled", BudgetTokens: budget}
	}
	return nil
}
