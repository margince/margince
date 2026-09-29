// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "testing"

func TestModelFamilyGroupsByLineWithOnlyTheStatedMerges(t *testing.T) {
	cases := map[string]string{
		"gemini-3.1-flash-lite":            "Gemini",
		"mistralai/ministral-14b-2512":     "Mistral", // the alias: ministral is Mistral's
		"mistralai/mistral-large-2512":     "Mistral",
		"google/gemma-4-31b-it":            "Gemma", // same publisher as Gemini, not the same family
		"gemma4:12b":                       "Gemma",
		"openai/gpt-oss-120b":              "GPT",
		"gpt-oss:20b":                      "GPT",
		"z-ai/glm-5.2":                     "GLM",
		"mlx-community/Qwen3-14B-4bit":     "Qwen", // the publisher is a repackager, the line is Qwen
		"anthropic/claude-haiku-4.5":       "Claude",
		"us.anthropic.claude-haiku-4-5-v1": "Claude",
		"":                                 OtherFamily,
		"4b-instruct":                      OtherFamily,
	}
	for model, want := range cases {
		if got := ModelFamily(model); got != want {
			t.Errorf("ModelFamily(%q) = %q, want %q", model, got, want)
		}
	}
}
