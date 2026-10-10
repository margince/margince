// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// annotate_brief writes an overnight pass's findings onto the acting rep's own
// brief run. The route and the tool resolve the same run from the same
// principal. Each pass replaces the findings of the one before, so a finding is
// read back after each door writes it.

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// briefItem reads one item of the brief as the route answers it, by the id the
// tool reads it under.
func (d *twoDoors) briefItem(t *testing.T, id string) map[string]json.RawMessage {
	t.Helper()
	var brief struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/brief", nil, http.StatusOK), &brief); err != nil {
		t.Fatalf("the brief does not decode: %v", err)
	}
	for _, item := range brief.Items {
		var got string
		if err := json.Unmarshal(item["id"], &got); err == nil && got == id {
			return item
		}
	}
	t.Fatalf("the brief no longer lists item %s", id)
	return nil
}

type briefItemRef struct {
	ItemID      string   `json:"item_id"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// A finding written onto a brief item over either door reads back the same: the
// same members appear on the item, with the same text.
func TestAnnotateBriefLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "brief-write-doors", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for _, name := range []string{"Brief deal one", "Brief deal two", "Brief deal three"} {
		createDealClosingThisWeek(t, d.e, stages, name)
	}
	if status := d.e.Call(t, "POST", "/v1/brief", nil, nil, nil); status != http.StatusCreated {
		t.Fatalf("generating the brief → %d", status)
	}
	var read struct {
		Items []briefItemRef `json:"items"`
	}
	if err := json.Unmarshal(d.tool(t, "read_brief", map[string]any{}), &read); err != nil || len(read.Items) != 3 {
		t.Fatalf("the brief holds %d items (%v), want the 3 seeded deals", len(read.Items), err)
	}
	finding := func(item briefItemRef) map[string]any {
		return map[string]any{"item_id": item.ItemID, "finding": "Quote went out Monday; chase Thursday", "cited_evidence": item.EvidenceIDs[:1]}
	}
	byRest, byTool, untouched := read.Items[0], read.Items[1], read.Items[2]

	d.restStatus(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "A quiet night", "items": []map[string]any{finding(byRest)},
	}, http.StatusNoContent)
	viaREST := d.briefItem(t, byRest.ItemID)
	d.tool(t, "annotate_brief", map[string]any{"narrative": "A quiet night", "items": []map[string]any{finding(byTool)}})
	viaTool := d.briefItem(t, byTool.ItemID)

	plain := d.briefItem(t, untouched.ItemID)
	var added []string
	for key := range viaREST {
		if _, held := plain[key]; !held {
			added = append(added, key)
		}
	}
	slices.Sort(added)
	if len(added) == 0 {
		t.Fatalf("an annotated item carries no member the untouched one lacks: %v", viaREST)
	}
	for _, key := range added {
		if got, want := string(viaTool[key]), string(viaREST[key]); got != want {
			t.Errorf("member %s reads %s on the tool's item and %s on the route's", key, got, want)
		}
	}
}
