// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"testing"
)

// viewsOfTwoShapes are a view that takes a list of ids and one that names a
// record by id in its path.
var viewsOfTwoShapes = []ReadView{
	{Name: "seat_names", Pattern: "/v1/users/names", Query: []ViewQuery{{Name: "id", Type: "array"}}},
	{
		Name: "company_360", Pattern: "/v1/companies/{id}/360", PathParams: []string{"id"},
		Query: []ViewQuery{{Name: "refresh", Type: "boolean"}, {Name: "metrics", Type: "array"}},
	},
}

// A list reaches its route as repeated values, and one value where the route
// takes one.
func TestReadViewPassesAListToARouteThatTakesOne(t *testing.T) {
	var gotPath map[string]string
	var gotQuery url.Values
	tool := readView{views: viewsOfTwoShapes, read: func(_ context.Context, _ ReadView, path map[string]string, query url.Values) (json.RawMessage, error) {
		gotPath, gotQuery = path, query
		return json.RawMessage(`{}`), nil
	}}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"view":"seat_names","id":["a","b"]}`)); err != nil {
		t.Fatalf("reading seat names: %v", err)
	}
	if !slices.Equal(gotQuery["id"], []string{"a", "b"}) {
		t.Errorf("the route was asked for ids %v, want both", gotQuery["id"])
	}
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"view":"company_360","id":"c1","refresh":true}`)); err != nil {
		t.Fatalf("reading a company: %v", err)
	}
	if gotPath["id"] != "c1" || gotQuery.Get("refresh") != "true" {
		t.Errorf("the route read id %q and refresh %q, want c1 and true", gotPath["id"], gotQuery.Get("refresh"))
	}
	var bad *BadArgsError
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"view":"company_360","id":["c1","c2"]}`)); !errors.As(err, &bad) {
		t.Errorf("a list for a path id answered %v, want a refused argument", err)
	}
}

// Each argument is published with its route's type. An argument one view
// takes as a list and another as one value says so.
func TestReadViewPublishesEachArgumentsType(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(readView{views: viewsOfTwoShapes}.Spec().InputSchema, &schema); err != nil {
		t.Fatalf("the schema does not decode: %v", err)
	}
	for arg, want := range map[string]string{"refresh": "boolean", "metrics": "array", "limit": "integer", "id": "string"} {
		if got := schema.Properties[arg].Type; got != want {
			t.Errorf("%s is published as %q, want %q", arg, got, want)
		}
	}
	if schema.Properties["id"].Description == "" {
		t.Error("id is a list for seat_names and one value elsewhere, and its description does not say so")
	}
}
