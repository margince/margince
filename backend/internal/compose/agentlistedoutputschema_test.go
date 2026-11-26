// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// tools/list carries a result shape only where a view reads the answer: the
// tool a view renders, and the tools a view acts through (actions.json). Every
// other tool keeps its schema in the server for the result check.
//
// The views are read off the served `_meta.ui.resourceUri`, and their count is
// held to the registry's, so a listing that lost a view binding fails.
func TestTheListingCarriesAResultShapeOnlyWhereAViewReadsIt(t *testing.T) {
	var served struct {
		Tools []mcpToolEntry `json:"tools"`
	}
	if err := json.Unmarshal(mcpResult(t, "tools/list", primedViews(t, everyDeclaredView())), &served); err != nil {
		t.Fatalf("reading tools/list: %v", err)
	}
	held := map[string]mcp.ToolSpec{}
	bound := 0
	for _, spec := range servedSurface(t).Specs() {
		held[spec.Name] = spec
		if spec.UI != nil && !spec.Unlisted {
			bound++
		}
	}
	actions := apps.ActionTools()
	views, dropped := 0, 0
	for _, tool := range served.Tools {
		listed := len(tool.OutputSchema) > 0
		_, actedThrough := actions[tool.Name]
		switch {
		case tool.Meta.UI != nil && tool.Meta.UI.ResourceURI != "":
			views++
			if !listed {
				t.Errorf("%s drives the view %s but tools/list carries no outputSchema for it",
					tool.Name, tool.Meta.UI.ResourceURI)
			}
		case actedThrough:
			if held[tool.Name].OutputSchema != nil && !listed {
				t.Errorf("%s is run from a view, which reads its answer, but tools/list carries no outputSchema", tool.Name)
			}
		case listed:
			t.Errorf("%s lists an outputSchema, but no view renders or calls it. Bind it to a view "+
				"(ToolSpec.UI) or add it to frontend/src/mcp-apps/actions.json; otherwise keep it in the server.",
				tool.Name)
		case held[tool.Name].OutputSchema != nil:
			dropped++
		}
	}
	if views != bound {
		t.Errorf("tools/list binds %d tools to a view and the registry declares %d, so this census is short", views, bound)
	}
	if views == 0 || dropped == 0 {
		t.Fatalf("%d view tools and %d server-held schemas were checked; with either at zero this test asserts nothing",
			views, dropped)
	}
}
