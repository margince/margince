// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// readTags and changeTags front the tag verbs, split by scope. merge_tags stays
// apart: it alone asks a human first, and a router fronts one tier.
type readTags struct{ routed }

func (readTags) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_tags", Title: "Read tags", Version: toolVersionV1,
		Description: readTagsCopy.render(),
		InputSchema: schema(`{"type":"object","properties":{
			"mode":{"type":"string","enum":["get","list","record"]}}}`),
	}
}

func (readTags) routeField() string { return "mode" }

func (readTags) routeTargets() map[string]string {
	return map[string]string{"list": "list_tags", "get": "get_tag", "record": "get_record_tags"}
}

type changeTags struct{ routed }

func (changeTags) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_tags", Title: "Change tags", Version: toolVersionV1,
		Description: changeTagsCopy.render(),
		Instead:     changeTagsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"action":{"type":"string","enum":["apply","create","remove","update"]}}}`),
	}
}

func (changeTags) routeField() string { return "action" }

func (changeTags) routeTargets() map[string]string {
	return map[string]string{"apply": "apply_tag", "remove": "remove_tag", "create": "create_tag", "update": "update_tag"}
}
