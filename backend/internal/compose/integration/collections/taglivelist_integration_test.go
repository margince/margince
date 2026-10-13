// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// A Live List whose filter names a tag that has since been merged away selects
// nothing, because an archived tag matches no record. The list says so, the way
// it does for a retired custom field, instead of reading as a healthy empty one.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type tagListWire struct {
	ID           string   `json:"id"`
	Health       string   `json:"health"`
	RetiredTags  []string `json:"retired_tags"`
	VisibleCount *int     `json:"visible_count"`
}

func TestALiveListOnAMergedAwayTagReportsItInsteadOfReadingHealthy(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithSchemaPool(integration.SchemaPool(t)), compose.WithListsEnabled(true))
	apptest.BootstrapWorkspaceSession(t, e, "Tag Live List", "admin@taglivelist.test", "Admin")
	source := createTag(t, e, "Keyaccount")
	target := createTag(t, e, "Key Account")
	other := createTag(t, e, "Prospect")
	createContactWithTag(t, e, "Carries Source", source)

	create := func(name, tag string) tagListWire {
		var list tagListWire
		if status := e.Call(t, "POST", "/v1/lists", integration.AnyMap{
			"name": name, "entity_type": "contact", "list_type": "dynamic",
			"definition": integration.AnyMap{"field": "tag", "op": "eq", "value": tag},
		}, nil, &list); status != http.StatusCreated {
			t.Fatalf("create list %q → %d", name, status)
		}
		return list
	}
	onSource, onOther := create("On source", source), create("On other", other)

	read := func(id string) tagListWire {
		var list tagListWire
		if status := e.Call(t, "GET", "/v1/lists/"+id, nil, nil, &list); status != http.StatusOK {
			t.Fatalf("read list → %d", status)
		}
		return list
	}
	if got := read(onSource.ID); got.Health != "ok" || got.RetiredTags != nil {
		t.Fatalf("before the merge the list reports %q %v, want ok", got.Health, got.RetiredTags)
	}

	if status := e.Call(t, "POST", "/v1/tags/"+source+"/merge",
		integration.AnyMap{"into_tag_id": target}, nil, nil); status != http.StatusOK {
		t.Fatalf("merge → %d", status)
	}

	got := read(onSource.ID)
	if got.Health != "retired_tag" || !slices.Equal(got.RetiredTags, []string{source}) {
		t.Errorf("after the merge the list reports health %q, retired_tags %v; want retired_tag naming %s",
			got.Health, got.RetiredTags, source)
	}
	if got := read(onOther.ID); got.Health != "ok" || got.RetiredTags != nil {
		t.Errorf("a list on a live tag reports %q %v, want ok", got.Health, got.RetiredTags)
	}
}
