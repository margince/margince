// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"github.com/margince/margince/backend/internal/shared/kernel/modelreply"
)

// ReasoningOutputMaxTokens is the output-token cap every structured
// model lane sets on a Request. It carries thinking headroom:
// a reasoning model (Gemini 3.x, o-series) spends output tokens on
// internal thinking BEFORE its answer, and that thinking counts against
// maxOutputTokens — so a cap sized for the answer alone starves the
// answer into a MAX_TOKENS stop with zero visible text. The failure is
// worst on the premium rung (e.g. gemini-3.5-flash), which every V1 task's
// ladder can escalate to. This value is generous enough for any V1 lane's
// answer plus that thinking, still small enough that a runaway completion
// terminates. The aicert lane's default candidate-completion cap derives
// from this same constant — one source for the reasoning-headroom ceiling.
const ReasoningOutputMaxTokens = 8192

// Unfence is kernel/modelreply.Unfence, re-exported.
//
// The reduction moved to shared/kernel so the agents module could reach it too
// (a module may not import a sibling). The name stays here because it is spelled
// at every model-reply parser in this tier, and re-pointing those reads as churn
// rather than as the change this was.
func Unfence(text string) string { return modelreply.Unfence(text) }
