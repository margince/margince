// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How much one site's requests think, when api/ai-tasks.yaml says so.
//
// A site's level is a FLOOR: think at least this much, and never less than the
// adapter would send without it — which on a structured Gemini request is
// already under the model's own default. It is a hint, not a guarantee: where
// the adapter cannot map it (a tool-carrying request, an unreadable model
// list) it is not sent. The router puts it on Request.ThinkingFloor and each
// adapter maps it to its own wire. Precedence, strongest first: the request's
// own ProviderOptions, then the binding's explicit setting (`thinking_level`,
// `routing.reasoning_effort`), then the site floor, then the adapter default.
// docs/reference/ai-thinking.md is the per-provider table.

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

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

// withSiteThinking returns req carrying its site's floor. A floor the caller
// set itself stands when the site declares none.
func withSiteThinking(req model.Request, task Task) (model.Request, error) {
	level, err := SiteThinking(task, req.Site)
	if err != nil || level == "" {
		return req, err
	}
	req.ThinkingFloor = level
	return req, nil
}

// thinkingFloorReaches reports whether binding's adapter maps a floor at all.
// An explicit binding setting outranks the floor, so such a binding takes none.
func thinkingFloorReaches(binding ProviderConfig) bool {
	d, _ := providerByName(binding.Provider)
	return d.thinkingFloor != nil && d.thinkingFloor(binding)
}

func geminiTakesThinkingFloor(binding ProviderConfig) bool {
	return binding.ThinkingLevel == "" && geminiTakesThinkingLevel(binding.Model)
}

func openRouterTakesThinkingFloor(binding ProviderConfig) bool {
	routing := UpstreamPreferencesFor(binding)
	return IsOpenRouterHost(binding.BaseURL) && (routing == nil || routing.ReasoningEffort == "")
}

func anthropicTakesThinkingFloor(binding ProviderConfig) bool {
	return anthropicThinkingModeOf(binding.Model) != anthropicThinksUnknown
}

func openaiTakesThinkingFloor(binding ProviderConfig) bool {
	_, reasons := openaiDefaultEffort(binding.Model)
	return reasons
}

// ollamaTakesThinkingFloor is true for every binding: whether the model thinks
// is /api/show's answer at call time, which a binding cannot see.
func ollamaTakesThinkingFloor(ProviderConfig) bool { return true }

// SiteThinkingLevels is, per site of task, the floor a request built for that
// site asks binding for; nil when no site declares one or binding takes none.
// It names what was ASKED, not what was sent: a model whose own default is
// already deeper is sent nothing, and still thought at least that much. An
// agent_loop site's requests carry tools, so it is left out where the adapter
// drops the floor on those.
func SiteThinkingLevels(binding ProviderConfig, task Task) map[string]string {
	if !thinkingFloorReaches(binding) {
		return nil
	}
	d, _ := providerByName(binding.Provider)
	return floorsAsked(taskSites[task], d.floorSkipsTools)
}

// floorsAsked is SiteThinkingLevels over sites, for an adapter that drops the
// floor on tool-carrying requests when skipsTools is set.
func floorsAsked(sites []Site, skipsTools bool) map[string]string {
	var levels map[string]string
	for _, site := range sites {
		if site.Thinking == "" || (skipsTools && site.Kind == SiteKindAgentLoop) {
			continue
		}
		if levels == nil {
			levels = map[string]string{}
		}
		levels[site.Name] = site.Thinking
	}
	return levels
}

// The thinking vocabulary every vendor's is a slice of.
const (
	effortNone    = "none"
	effortMinimal = "minimal"
	effortLow     = "low"
	effortMedium  = "medium"
	effortHigh    = "high"
	effortXHigh   = "xhigh"
	effortMax     = "max"
)

// effortRank orders a thinking level on the one scale every vendor's
// vocabulary is a slice of: reasoningEfforts, hardest first.
func effortRank(level string) (int, bool) {
	i := slices.Index(reasoningEfforts, level)
	return len(reasoningEfforts) - i, i >= 0
}

// effortAtLeast reports whether level is known and no shallower than floor.
func effortAtLeast(level, floor string) bool {
	have, ok := effortRank(level)
	want, wantOK := effortRank(floor)
	return ok && wantOK && have >= want
}

// lowestEffortAtLeast is the shallowest of offered that still meets floor,
// empty when none does. `none` never meets a floor.
func lowestEffortAtLeast(floor string, offered []string) string {
	best, bestRank := "", len(reasoningEfforts)+1
	for _, level := range offered {
		rank, _ := effortRank(level)
		if level != effortNone && effortAtLeast(level, floor) && rank < bestRank {
			best, bestRank = level, rank
		}
	}
	return best
}
