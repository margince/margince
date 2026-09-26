// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How deeply one site's requests think, when api/ai-tasks.yaml says so.
//
// Precedence, strongest first: the request's own
// ProviderOptions["gemini"].thinking_level, then the site's declared
// `thinking`, then the binding's `thinking_level`, then the adapter's default
// (geminithinking.go). The site beats the binding because the evidence is per
// prompt: one model thinks its way out of a failure on one site and answers
// another site worse for it, and a binding is shared by every site on the rung.
//
// Only a Gemini 3 rung is sent it. The OpenAI and Ollama wires carry a level
// too, but neither binding says whether its model can think at all, and a
// level sent to one that cannot fails every call on that rung.

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// geminiThinkingOption is the key the gemini adapter reads a request's own
// level from, inside its ProviderOptions namespace.
const geminiThinkingOption = "thinking_level"

// SiteThinking returns the thinking level task's site declares, empty when it
// declares none or the request names no site. A site the task never declares
// is an error: the request was built for a prompt the contract does not know,
// and a hint silently dropped would ship the failure the hint exists to fix.
func SiteThinking(task Task, site string) (string, error) {
	if site == "" {
		return "", nil
	}
	for _, declared := range taskSites[task] {
		if declared.Name == site {
			return declared.Thinking, nil
		}
	}
	return "", fmt.Errorf("ai: task %s declares no site %q; name one of its sites in api/ai-tasks.yaml or leave Request.Site empty", task, site)
}

// siteThinkingReaches reports whether a rung is sent a site's level at all.
func siteThinkingReaches(provider, modelID string) bool {
	return provider == providerGemini && geminiTakesThinkingLevel(modelID)
}

// SiteThinkingLevels is, per site of task, the level a request built for that
// site is served at on binding, wherever it differs from the binding's own
// level. It is the router's precedence read for a request that names no level
// of its own, which is how every site builder builds one; nil means every site
// runs at the binding's level.
func SiteThinkingLevels(binding ProviderConfig, task Task) map[string]string {
	if !siteThinkingReaches(binding.Provider, binding.Model) {
		return nil
	}
	var levels map[string]string
	for _, site := range taskSites[task] {
		if site.Thinking == "" || site.Thinking == binding.ThinkingLevel {
			continue
		}
		if levels == nil {
			levels = map[string]string{}
		}
		levels[site.Name] = site.Thinking
	}
	return levels
}

// withSiteThinking returns req as it is sent to one rung: carrying its site's
// level in the gemini namespace, unless the request names a level of its own
// or the rung is not sent one. The caller's map is never written to — the
// same request walks every rung of the ladder.
func withSiteThinking(req model.Request, task Task, lane routeMeta) (model.Request, error) {
	level, err := SiteThinking(task, req.Site)
	if err != nil || level == "" || !siteThinkingReaches(lane.provider, lane.model) {
		return req, err
	}
	own, err := geminiReadOptions(req.ProviderOptions)
	if err != nil || own.ThinkingLevel != "" {
		return req, err
	}
	fields := map[string]json.RawMessage{}
	if raw := req.ProviderOptions[providerGemini]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return req, fmt.Errorf("ai: gemini: provider options: %w", err)
		}
	}
	encodedLevel, err := json.Marshal(level)
	if err != nil {
		return req, fmt.Errorf("ai: site thinking level: %w", err)
	}
	fields[geminiThinkingOption] = encodedLevel
	namespace, err := json.Marshal(fields)
	if err != nil {
		return req, fmt.Errorf("ai: gemini: provider options: %w", err)
	}
	options := maps.Clone(req.ProviderOptions)
	if options == nil {
		options = map[string]json.RawMessage{}
	}
	options[providerGemini] = namespace
	req.ProviderOptions = options
	return req, nil
}
