// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// A prompt is offered only when every tool it names is, so a renamed tool hides
// its prompt without an error anywhere. Over the composed surface a caller with
// every scope must be offered the whole catalog.
func TestEveryStarterPromptIsOfferedOverTheComposedSurface(t *testing.T) {
	var answer struct {
		Prompts []struct {
			Name string `json:"name"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(mcpResult(t, "prompts/list", nil), &answer); err != nil {
		t.Fatalf("decoding prompts/list: %v", err)
	}
	var offered []string
	for _, prompt := range answer.Prompts {
		offered = append(offered, prompt.Name)
	}
	for _, name := range agents.StarterPromptNames() {
		if !slices.Contains(offered, name) {
			t.Errorf("starter prompt %s is not offered over the composed surface: it names a tool the registry does not serve (offered: %v)", name, offered)
		}
	}
}
