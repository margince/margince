// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A listing carries every registered tool that a description or an input schema
// names. A surface that offers part of the catalog can then drop the sentence.
func TestASpecListsTheRegisteredToolsItsTextNames(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil, nil)
	register := func(name, description, schema string) {
		r.Register(&fakeTool{spec: mcp.ToolSpec{
			Name: name, Title: name, Version: testToolVersion,
			Description: describedForRegistration + " " + description, Tier: mcp.TierAutoExecute,
			InputSchema: json.RawMessage(schema),
		}})
	}
	register("read_a", "Plain.", `{"type":"object"}`)
	register("read_b", "Plain.", `{"type":"object"}`)
	register("read_c", "Use read_a for names; read_c is this tool.", `{"type":"object","properties":{"x":{"description":"read_b lists them."}}}`)
	var got []string
	for _, spec := range r.Specs() {
		if spec.Name == "read_c" {
			got = spec.NamedTools
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"read_a", "read_b"}) {
		t.Errorf("NamedTools = %v, want [read_a read_b]: the tool itself is no neighbour", got)
	}
}
