// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// What an openai_compatible binding sends differently when its target is
// OpenAI itself. Gateways and vLLM take max_tokens and know no reasoning_effort,
// so the OpenAI-only fields go out only where OpenAI's own API will read them.

import (
	"net/url"
	"strings"
)

const openAIHost = "api.openai.com"

// pointsAtOpenAI reports whether baseURL is OpenAI's own API, parsed rather
// than substring-matched so a lookalike host does not receive OpenAI-only fields.
func pointsAtOpenAI(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && sendableHTTPScheme(parsed) && strings.EqualFold(parsed.Hostname(), openAIHost)
}

// sendsMaxCompletionTokens reports whether the output ceiling goes out as
// max_completion_tokens. OpenAI deprecates max_tokens and refuses it for o-series
// models, so its own host always gets the new name, and so does a bare reasoning
// model id (o3, gpt-5-mini) behind a gateway that fronts OpenAI, such as Azure.
// Every other model keeps max_tokens, the name vLLM and brokers read.
func sendsMaxCompletionTokens(baseURL, modelID string) bool {
	if pointsAtOpenAI(baseURL) {
		return true
	}
	_, reasons := openaiDefaultEffort(modelID)
	return reasons
}

// chatReasoningEffort is the reasoning_effort a request's admin level or floor
// sends on OpenAI's own chat wire, from the table the Responses wire uses.
func chatReasoningEffort(baseURL, modelID, level, floor string) string {
	if !pointsAtOpenAI(baseURL) {
		return ""
	}
	return openaiThinkingEffort(modelID, level, floor)
}
