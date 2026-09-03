// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// What an OpenAI-wire failure is allowed to say.
//
// Split from openaicompat.go because it answers a different question from the
// rest of that file: not "how is a request sent and a response read" but "how
// much of a REMOTE party's words may this installation write into its own
// logs". Every function here is a narrowing — the vendor's structured message
// only, never the raw body; the upstream's sentence rather than a broker's
// generic one; redacted through the same stripper the payloads use; and capped.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// openAICompatError surfaces the vendor's structured error message only —
// never the raw response body, which may be unstructured HTML/text — so a logged
// failure can't echo the request or leak provider internals (the anthropic /
// openai pattern). Three structured shapes exist on this generic wire: OpenAI's
// nested {"error":{type,message}}, vLLM's top-level {"object":"error", type,
// message}, and a broker's {"error":{message,metadata:{raw,provider_name}}};
// a body that can't be read falls back to the HTTP status.
//
// The broker shape is the reason metadata is read at all. A gateway in front of
// several vendors answers with its OWN message — OpenRouter says "Provider
// returned error" for everything — and puts the upstream vendor's sentence in
// metadata.raw. Reading only the outer message produced log lines that named
// no cause and a build failure the operator could do nothing with.
func openAICompatError(resp *http.Response) error {
	var apiErr struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			Metadata struct {
				Raw          string `json:"raw"`
				ProviderName string `json:"provider_name"`
				LimitSource  string `json:"limit_source"`
			} `json:"metadata"`
		} `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr == nil && json.Unmarshal(raw, &apiErr) == nil {
		if detail := compatErrorDetail(apiErr.Error.Message, apiErr.Error.Metadata.Raw, apiErr.Error.Metadata.ProviderName); detail != "" {
			return providerRefusal(resp, apiErr.Error.Metadata.LimitSource,
				fmt.Errorf("ai: openai-compat: %s: %s (http %d)", safeProviderText(apiErr.Error.Type), detail, resp.StatusCode))
		}
		if apiErr.Message != "" {
			return providerRefusal(resp, "", fmt.Errorf("ai: openai-compat: %s: %s (http %d)",
				safeProviderText(apiErr.Type), safeProviderText(apiErr.Message), resp.StatusCode))
		}
	}
	return providerRefusal(resp, "", fmt.Errorf("ai: openai-compat: http %d", resp.StatusCode))
}

// compatErrorDetail is the sentence worth logging out of a broker's answer.
//
// The upstream vendor's own words win when there are any: a broker's outer
// message is written about ITS call to the vendor, not about the request, so
// it is the same string whatever went wrong. The vendor is named alongside,
// because "rate-limited upstream" means nothing without saying whose limit.
//
// Both are text a REMOTE party chose, so neither is logged as it arrived.
// An upstream that echoes a prompt, a URL with a token in it, or its own
// internals would otherwise write all of it into this installation's logs
// through a path nothing else guards. Redacted through the same stripper the
// model payloads use, and capped: a vendor's cause is one sentence, and
// anything longer is a body that lost its way into a message field.
func compatErrorDetail(message, upstreamRaw, providerName string) string {
	upstreamRaw = strings.TrimSpace(upstreamRaw)
	if upstreamRaw == "" {
		return safeProviderText(message)
	}
	if providerName == "" {
		return safeProviderText(upstreamRaw)
	}
	return safeProviderText(providerName + ": " + upstreamRaw)
}

// providerTextMax bounds one logged vendor sentence.
const providerTextMax = 300

// safeProviderText redacts and bounds text a remote provider chose.
//
// Applied to the `type` discriminator as well as the message, which is not
// obvious: a type reads like a short enum an API author picked, and on a broker
// it is a string an upstream chose, so it can be long or carry the request back.
// Every remote field on this path goes through here or none of it is worth
// having.
func safeProviderText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	stripped, _, err := NewSecretStripper().Strip(context.Background(), []byte(text))
	if err != nil {
		// The stripper could not vouch for it, so none of it is logged: a
		// vendor sentence is worth having, never at the price of writing an
		// unexamined remote string into the log.
		return "(provider detail withheld)"
	}
	text = strings.Join(strings.Fields(string(stripped)), " ")
	if len(text) > providerTextMax {
		return text[:providerTextMax] + "…"
	}
	return text
}
