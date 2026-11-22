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

// azureOpenAIHostSuffix marks Azure OpenAI by its host, the one positive
// identification a binding gives.
const azureOpenAIHostSuffix = ".openai.azure.com"

// sendsMaxCompletionTokens reports whether the output ceiling goes out as
// max_completion_tokens. OpenAI refuses max_tokens for o-series models, and
// Azure OpenAI does too. Both are identified by host alone: a model id says
// nothing about the server, and a vLLM or LiteLLM deployment named o3 reads
// max_tokens. Every other host keeps max_tokens.
func sendsMaxCompletionTokens(baseURL string) bool {
	if pointsAtOpenAI(baseURL) {
		return true
	}
	parsed, err := url.Parse(baseURL)
	return err == nil && sendableHTTPScheme(parsed) && strings.HasSuffix(strings.ToLower(parsed.Hostname()), azureOpenAIHostSuffix)
}

// chatReasoningEffort is the reasoning_effort a request's admin level or floor
// sends on OpenAI's own chat wire, from the table the Responses wire uses.
func chatReasoningEffort(baseURL, modelID, level, floor string) string {
	if !pointsAtOpenAI(baseURL) {
		return ""
	}
	return openaiThinkingEffort(modelID, level, floor)
}
