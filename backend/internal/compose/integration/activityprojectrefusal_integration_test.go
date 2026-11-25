// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
)

// projectFilingCode is the refusal the activity store gives an agent filing a
// new activity under a project.
const projectFilingCode = "project_filing_needs_approval"

// projectLinkArgs is, per change_activities action whose links offer a project,
// the smallest call it otherwise runs, so a refusal can only be the project's.
var projectLinkArgs = map[string]map[string]any{
	"log":  {"kind": "note", "subject": "Filed under the rollout"},
	"task": {"subject": "Follow up on the rollout", "body": "Dana asked for the plan on the call."},
	"check_new_send": {"request_body": AnyMap{
		"to": []string{"dana@rollout.test"}, "subject": "Rollout", "body": "The plan, as promised.",
	}},
}

// One links shape serves every change_activities action, so each action whose
// project link the server refuses must say so, and only those.
func TestEveryActivityActionSaysWhetherItRefusesAProject(t *testing.T) {
	d := newTwoDoors(t, "activity-project-refusal", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Rollout Customer", "source": "manual"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Rollout", "company_id": company, "source": "manual"})
	links := []AnyMap{{"entity_type": "project", "entity_id": project}}

	offering := projectLinkActions(t, d)
	if len(offering) == 0 {
		t.Fatal("no change_activities action offers a project link, so this test reads nothing")
	}
	for _, action := range slices.Sorted(maps.Keys(offering)) {
		args, ok := projectLinkArgs[action]
		if !ok {
			t.Errorf("%s offers a project link and projectLinkArgs has no call for it", action)
			continue
		}
		call := maps.Clone(args)
		call["action"] = action
		if body, nested := call["request_body"].(AnyMap); nested {
			body = maps.Clone(body)
			body["links"] = links
			call["request_body"] = body
		} else {
			call["links"] = links
		}
		got := d.mcp.Call(t, "change_activities", call)
		refused := got.IsError && strings.Contains(got.Text, projectFilingCode)
		if got.IsError && !refused {
			t.Errorf("%s failed for another reason, so it says nothing about a project:\n%s", action, got.Text)
			continue
		}
		if says := strings.Contains(offering[action], projectFilingCode); says != refused {
			t.Errorf("%s: refused=%t, but its links description says %q", action, refused, offering[action])
		}
	}
}

// projectLinkActions reads the action catalog for the change_activities actions
// whose links take a project, with what each one says of its links.
func projectLinkActions(t *testing.T, d *twoDoors) map[string]string {
	t.Helper()
	var catalog struct {
		Actions []struct {
			Action string `json:"action"`
			Body   struct {
				Properties struct {
					Links *struct {
						Description string `json:"description"`
						Items       struct {
							Properties struct {
								EntityType struct {
									Enum []string `json:"enum"`
								} `json:"entity_type"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"links"`
				} `json:"properties"`
			} `json:"body"`
		} `json:"actions"`
	}
	raw := d.tool(t, "read_workspace", map[string]any{"topic": "actions", "router": "change_activities"})
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("the action catalog does not decode: %v\n%s", err, raw)
	}
	out := map[string]string{}
	for _, a := range catalog.Actions {
		links := a.Body.Properties.Links
		if links != nil && slices.Contains(links.Items.Properties.EntityType.Enum, "project") {
			out[a.Action] = links.Description
		}
	}
	return out
}
