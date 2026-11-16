// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Thinking on the Anthropic wire, shaped by Anthropic's per-model table. 4.6 to
// 4.8 take `thinking: {type: "adaptive"}` and think only when asked. 4.5 and
// earlier take only `{type: "enabled", budget_tokens: N}`. 5.x always think and
// reject both shapes; `output_config.effort` sets their depth. That default is
// medium on Opus 5.5 and Haiku 5.5, and high on the rest. A floor sends none,
// since every floor is at or under each default; an admin's exact level is sent.

import "strings"

type anthropicThinkingMode int

const (
	anthropicThinksUnknown anthropicThinkingMode = iota
	// anthropicThinksByDefault is a model that thinks unasked; its depth is
	// output_config.effort, never a thinking block.
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
	{"claude-haiku-5", anthropicThinksByDefault},
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

// anthropicAnswerRoom is the least of max_tokens a thinking budget must leave
// the answer, so a floor never cuts short a reply that fitted without it.
const anthropicAnswerRoom = 1024

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// anthropicThinkingFor is the `thinking` block a request is sent, nil for none.
//
// A request carrying tools is sent none: a thinking turn that calls a tool must
// be replayed with its thinking block, and this adapter keeps text only. A
// budget that would leave the answer less than anthropicAnswerRoom of
// max_tokens is not sent either: thinking may spend its whole budget, and the
// floor is not worth a cut answer.
func anthropicThinkingFor(modelID, floor string, maxTokens, tools int) *anthropicThinking {
	if floor == "" || tools > 0 {
		return nil
	}
	switch anthropicThinkingModeOf(modelID) {
	case anthropicThinksAdaptive:
		return &anthropicThinking{Type: "adaptive"}
	case anthropicThinksWithBudget:
		budget, ok := anthropicThinkingBudgets[floor]
		if !ok || maxTokens-budget < anthropicAnswerRoom {
			return nil
		}
		return &anthropicThinking{Type: "enabled", BudgetTokens: budget}
	}
	return nil
}

// anthropicEffortFor is the output_config.effort a request is sent, "" for
// none: only an admin's exact level, and only to a model that thinks by
// default. The wire has no minimal, so minimal asks for its lowest, low.
func anthropicEffortFor(modelID, level string) string {
	if level == "" || anthropicThinkingModeOf(modelID) != anthropicThinksByDefault {
		return ""
	}
	if level == effortMinimal {
		return effortLow
	}
	return level
}
