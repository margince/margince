// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// An agent learns the record kinds, link types and company types through
// read_workspace, and is answered what the route answers its user. A link type
// it learns there is a value list_records on deals accepts.
func TestAnAgentDiscoversExtensionValuesAsItsUserDoes(t *testing.T) {
	d := newTwoDoors(t, "extension-discovery", "read")
	type value struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	type directory struct {
		Data []value `json:"data"`
	}
	for topic, path := range map[string]string{
		"record_kinds": "/v1/record-kinds", "link_types": "/v1/link-types", "company_types": "/v1/company-types",
	} {
		sameRecords[directory](t, topic, d.rest(t, "GET", path, nil), d.tool(t, "read_workspace", map[string]any{"topic": topic}))
	}
	var linkTypes directory
	if err := json.Unmarshal(d.tool(t, "read_workspace", map[string]any{"topic": "link_types"}), &linkTypes); err != nil {
		t.Fatal(err)
	}
	if len(linkTypes.Data) == 0 {
		t.Fatal("the agent was told of no link type")
	}
	d.tool(t, "list_records", map[string]any{"record_type": "deal", "filters": map[string]any{"link_type": linkTypes.Data[0].Name}})
}
