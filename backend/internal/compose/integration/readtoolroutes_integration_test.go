// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Read tools whose route keeps its own shape. Each test seeds through the
// product's writers and asks the route as a passport and the tool over MCP.
// It compares the records both answers name.

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

var uuidInDocument = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// documentRecords answers the distinct record ids a document names, sorted. A
// route and its tool may wrap and label a record differently; the ids they
// carry are what must agree.
func documentRecords(raw json.RawMessage) []string {
	found := uuidInDocument.FindAllString(string(raw), -1)
	slices.Sort(found)
	return slices.Compact(found)
}

// sameDoorRecords fails unless both doors name the same records and every
// wanted id is among them. An empty answer fails too, so a door that finds
// nothing cannot pass by agreeing with the other.
func sameDoorRecords(t *testing.T, what string, rest, tool json.RawMessage, want ...string) {
	t.Helper()
	fromREST, fromTool := documentRecords(rest), documentRecords(tool)
	if len(fromREST) == 0 {
		t.Fatalf("%s: the route named no record:\n%s", what, rest)
	}
	if !slices.Equal(fromREST, fromTool) {
		t.Errorf("%s: the route names %v, the tool names %v\nroute: %s\ntool: %s", what, fromREST, fromTool, rest, tool)
	}
	for _, id := range want {
		if !slices.Contains(fromREST, id) {
			t.Errorf("%s: the seeded record %s is missing from the route's answer", what, id)
		}
	}
}

// bothDoorsName fails unless each wanted record is named in both answers.
// It serves answers where one door also carries ids of its own, such as an
// actor's user id, so the whole sets cannot be equal.
func bothDoorsName(t *testing.T, what string, rest, tool json.RawMessage, want ...string) {
	t.Helper()
	if len(want) == 0 {
		t.Fatalf("%s: no record is wanted, so nothing is compared", what)
	}
	for _, id := range want {
		if !slices.Contains(documentRecords(rest), id) {
			t.Errorf("%s: the route does not name %s:\n%s", what, id, rest)
		}
		if !slices.Contains(documentRecords(tool), id) {
			t.Errorf("%s: the tool does not name %s:\n%s", what, id, tool)
		}
	}
}

// sameDocument fails unless both doors answer one JSON document. A `data`
// envelope on one door only is set aside.
func sameDocument(t *testing.T, what string, rest, tool json.RawMessage) {
	t.Helper()
	if !reflect.DeepEqual(decoded(t, unwrapData(rest, tool)), decoded(t, unwrapData(tool, rest))) {
		t.Errorf("%s: the doors answered different documents\nroute: %s\ntool: %s", what, rest, tool)
	}
}

// decoded reads a document into generic JSON, so member order does not matter.
//
//craft:ignore naked-any a document under comparison is whichever JSON the door answered
func decoded(t *testing.T, doc json.RawMessage) any {
	t.Helper()
	var out any
	if err := json.Unmarshal(doc, &out); err != nil {
		t.Fatalf("an answer does not decode: %v\n%s", err, doc)
	}
	return out
}

// unwrapData answers doc without its single top-level `data` member when the
// other answer has no such member.
func unwrapData(doc, other json.RawMessage) json.RawMessage {
	var members map[string]json.RawMessage
	var otherMembers map[string]json.RawMessage
	if json.Unmarshal(doc, &members) != nil || json.Unmarshal(other, &otherMembers) != nil {
		return doc
	}
	inner, wrapped := members["data"]
	if _, otherWraps := otherMembers["data"]; wrapped && !otherWraps && len(members) == 1 {
		return inner
	}
	return doc
}

// memberOf answers one member of a tool's answer, for a tool that names the
// question it answered.
func memberOf(t *testing.T, answer json.RawMessage, member string) json.RawMessage {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal(answer, &members); err != nil || members[member] == nil {
		t.Fatalf("the answer has no %q member:\n%s", member, answer)
	}
	return members[member]
}

// A contact, a company and a lead read the same on GET /<type>/{id} and
// read_record, and list the same on GET /<type> and list_records.
func TestRecordReadAndListRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "record-read-doors", "read")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Door Works"})
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Doris Door"})
	lead := createdRecord(t, d.e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Lena Lead"})

	for _, c := range []struct{ kind, route, id string }{
		{"company", "companies", company}, {"contact", "contacts", contact}, {"lead", "leads", lead},
	} {
		sameDoorRecords(t, "read "+c.kind,
			d.rest(t, "GET", "/v1/"+c.route+"/"+c.id, nil),
			d.tool(t, "read_record", map[string]any{"record_type": c.kind, "id": c.id}), c.id)
		sameDoorRecords(t, "list "+c.kind,
			d.rest(t, "GET", "/v1/"+c.route+"?limit=10", nil),
			d.tool(t, "list_records", map[string]any{"record_type": c.kind, "limit": 10}), c.id)
	}
}

// GET /search and search_records find the same records for one query, with and
// without naming a type.
func TestSearchRouteAnswersAsSearchRecords(t *testing.T) {
	d := newTwoDoors(t, "search-records-doors", "read")
	hit := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Quentin Zephyrwind"})
	other := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Bernd Unrelated"})

	// The tool also names a hit's owner. So the answers are compared on the
	// hit and on the record neither may return.
	for _, c := range []struct {
		what  string
		route string
		args  map[string]any
	}{
		{"search across types", "/v1/search?q=Zephyrwind&limit=10", map[string]any{"q": "Zephyrwind", "limit": 10}},
		{
			"search one type", "/v1/search?q=Zephyrwind&record_type=contact&limit=10",
			map[string]any{"q": "Zephyrwind", "record_type": "contact", "limit": 10},
		},
	} {
		rest, tool := d.rest(t, "GET", c.route, nil), d.tool(t, "search_records", c.args)
		bothDoorsName(t, c.what, rest, tool, hit)
		for door, answer := range map[string]json.RawMessage{"route": rest, "tool": tool} {
			if slices.Contains(documentRecords(answer), other) {
				t.Errorf("%s: the %s returns the contact the query does not match", c.what, door)
			}
		}
	}
}

// A tag, the tags list and a record's tags read the same on their routes and
// tools.
func TestTagRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "tag-read-doors", "read")
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Tamsin Tagged"})
	tag := createdID(t, d.e, "/v1/tags", AnyMap{"name": "Door champion"})
	createdID(t, d.e, "/v1/tags", AnyMap{"name": "Door other"})
	if status := d.e.Call(t, "POST", "/v1/tags/"+tag+"/apply",
		AnyMap{"entity_type": "contact", "entity_id": contact}, nil, nil); status != 201 {
		t.Fatalf("tagging the contact → %d", status)
	}

	sameDoorRecords(t, "one tag",
		d.rest(t, "GET", "/v1/tags/"+tag, nil), d.tool(t, "get_tag", map[string]any{"tag_id": tag}), tag)
	sameDoorRecords(t, "the tags list",
		d.rest(t, "GET", "/v1/tags", nil), d.tool(t, "list_tags", map[string]any{}), tag)
	bothDoorsName(t, "a record's tags",
		d.rest(t, "GET", "/v1/records/contact/"+contact+"/tags", nil),
		d.tool(t, "get_record_tags", map[string]any{"record_type": "contact", "record_id": contact}), tag)
}

// A project's page reads the same on GET /projects/{id}/360 and read_project_360.
func TestProjectPageRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "project-page-doors", "read")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Door Projects"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Door rollout", "company_id": company, "source": "manual"})

	bothDoorsName(t, "project page",
		d.rest(t, "GET", "/v1/projects/"+project+"/360", nil),
		d.tool(t, "read_project_360", map[string]any{"project_id": project}), project, company)
}

// The roster and the transports read the same on GET /users and
// list_colleagues, and on GET /channel-providers and list_channel_providers.
func TestRosterAndTransportRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "roster-doors", "read")

	sameDoorRecords(t, "colleagues",
		d.rest(t, "GET", "/v1/users", nil), d.tool(t, "list_colleagues", map[string]any{}))

	rest := d.rest(t, "GET", "/v1/channel-providers", nil)
	tool := d.tool(t, "list_channel_providers", map[string]any{})
	if got, want := providerNames(t, rest), providerNames(t, tool); len(got) == 0 || !slices.Equal(got, want) {
		t.Errorf("the route lists transports %v, the tool %v", got, want)
	}
}

// providerNames reads the provider keys out of either door's answer, whichever
// member the list sits under.
func providerNames(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var wrapped struct {
		Data      []struct{ Provider string } `json:"data"`
		Providers []struct{ Provider string } `json:"providers"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		t.Fatalf("the provider list does not decode: %v\n%s", err, raw)
	}
	var names []string
	for _, p := range append(wrapped.Data, wrapped.Providers...) {
		names = append(names, p.Provider)
	}
	slices.Sort(names)
	return names
}
