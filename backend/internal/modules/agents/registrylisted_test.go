// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A listed description naming a verb only a router reaches names the router
// call instead, in its text and its arguments. A word the tool takes as one of
// its own values keeps its spelling, and a verb no router fronts is untouched.
func TestTheListingNamesAFrontedVerbByItsRouter(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(&argsTool{spec: targetSpec("fake_a", principal.ScopeWrite, mcp.TierAutoExecute)})
	r.Register(fakeRouter{name: "fake_router", targets: map[string]string{"go": "fake_a"}})
	neighbour := mcp.ToolSpec{
		Name: "neighbour", Title: "neighbour", Version: testToolVersion, RequiredScope: principal.ScopeWrite,
		Description: "Use fake_a for the other errand, or fake_b, which stays listed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"note":{"type":"string","description":"what fake_a answered"}}}`),
	}
	got := r.Listed(neighbour)
	if want := "Use fake_router with action=go for the other errand, or fake_b, which stays listed."; got.Description != want {
		t.Errorf("the description reads %q, want %q", got.Description, want)
	}
	if !strings.Contains(string(got.InputSchema), "what fake_router with action=go answered") {
		t.Errorf("an argument's description still names the hidden verb: %s", got.InputSchema)
	}

	bulk := mcp.ToolSpec{
		Name: "bulk", Title: "bulk", Version: testToolVersion, RequiredScope: principal.ScopeWrite,
		Description: "Runs fake_a on many records.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"verb":{"type":"string","enum":["fake_a","other"]}}}`),
	}
	if kept := r.Listed(bulk); kept.Description != bulk.Description || string(kept.InputSchema) != string(bulk.InputSchema) {
		t.Errorf("a tool taking fake_a as its own value was reworded: %q %s", kept.Description, kept.InputSchema)
	}
}
