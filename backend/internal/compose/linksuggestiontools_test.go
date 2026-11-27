// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"
)

// The link suggestion queue reaches an agent as three router actions, and the
// read is a choice in the topic list read_workspace offers a client.
func TestTheLinkSuggestionQueueIsServedOnItsRouters(t *testing.T) {
	want := map[string]map[string]string{
		"read_workspace":    {"link_suggestions": "list_link_suggestions"},
		"change_activities": {"accept_link_suggestion": "accept_link_suggestion", "reject_link_suggestion": "reject_link_suggestion"},
	}
	found := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		actions, routed := want[spec.Name]
		if !routed {
			continue
		}
		found++
		for _, action := range spec.Actions {
			if verb, ok := actions[action.Action]; ok {
				if action.Verb != verb {
					t.Errorf("%s.%s runs %s, want %s", spec.Name, action.Action, action.Verb, verb)
				}
				delete(actions, action.Action)
			}
		}
		if len(actions) > 0 {
			t.Errorf("%s does not offer %v", spec.Name, actions)
		}
		if spec.Name != "read_workspace" {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatalf("read_workspace's input schema does not read: %v", err)
		}
		if spec.ActionField != "topic" || !slices.Contains(schema.Properties["topic"].Enum, "link_suggestions") {
			t.Errorf("read_workspace picks by %q and its topic list is %v, want link_suggestions among the topics",
				spec.ActionField, schema.Properties["topic"].Enum)
		}
	}
	if found != len(want) {
		t.Fatalf("found %d of the %d routers, want both", found, len(want))
	}
}
