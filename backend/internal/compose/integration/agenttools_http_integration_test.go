// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type agentToolWire struct {
	Name          string `json:"name"`
	RequiredScope string `json:"required_scope"`
	Tier          string `json:"tier"`
	Egress        bool   `json:"egress"`
	Actions       *[]struct {
		Verb string `json:"verb"`
		Tier string `json:"tier"`
	} `json:"actions"`
}
type agentToolListWire struct {
	Data []agentToolWire `json:"data"`
}

func TestListAgentToolsMirrorsTheGovernedSurface(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var page agentToolListWire
	status := e.Call(t, "GET", "/v1/agent-tools", nil, nil, &page)
	if status != http.StatusOK {
		t.Fatalf("GET /agent-tools status = %d, want 200", status)
	}
	if len(page.Data) == 0 {
		t.Fatal("expected a non-empty governed tool surface")
	}
	var found bool
	for _, tool := range page.Data {
		// search_records is reached through find_records, and its row carries the
		// verb with the verb's own tier.
		if tool.Name == "find_records" && tool.Actions != nil {
			for _, action := range *tool.Actions {
				if action.Verb == "search_records" {
					found = true
					if action.Tier != "auto_execute" || tool.Egress {
						t.Fatalf("find_records' search action = %+v on %+v, want auto_execute/non-egress", action, tool)
					}
				}
			}
		}
		if tool.Tier != "auto_execute" && tool.Tier != "confirmation_required" && tool.Tier != "dynamic" {
			t.Fatalf("tool %q has unmapped tier %q", tool.Name, tool.Tier)
		}
	}
	if !found {
		t.Fatal("search_records is missing from the surface, as a tool and as a router's action")
	}
}
