// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A scheduled agent's run is offered what the registry offers. An Unlisted
// tool is never offered, so an allowlist naming one leaves the run without it.
func TestNoScheduledAgentIsGivenAnUnlistedTool(t *testing.T) {
	unlisted := unlistedTools(t)
	for _, agent := range mustScheduledAgents() {
		for _, name := range agent.Tools {
			if unlisted[name] {
				t.Errorf("%s is allowed %s, which only a router reaches; allow the router instead",
					agent.Name, name)
			}
		}
	}
}

// A model can call only what it is shown. A description that names an Unlisted
// tool sends the model to a name it was never given.
func TestNoOfferedDescriptionNamesAnUnlistedTool(t *testing.T) {
	unlisted := unlistedTools(t)
	for _, spec := range servedSurface(t).Specs() {
		if spec.Unlisted {
			continue
		}
		for _, word := range strings.FieldsFunc(spec.Description, func(c rune) bool {
			return c != '_' && !unicode.IsLower(c) && !unicode.IsDigit(c)
		}) {
			if unlisted[word] {
				t.Errorf("%s's description names %s, which is not offered; name the router that fronts it",
					spec.Name, word)
			}
		}
	}
}

// offeredSpecs is the served surface a client is offered: every registered tool
// but the ones only a router reaches.
func offeredSpecs(t *testing.T) []mcp.ToolSpec {
	t.Helper()
	return slices.DeleteFunc(servedSurface(t).Specs(), func(spec mcp.ToolSpec) bool { return spec.Unlisted })
}

func unlistedTools(t *testing.T) map[string]bool {
	t.Helper()
	unlisted := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		if spec.Unlisted {
			unlisted[spec.Name] = true
		}
	}
	return unlisted
}
