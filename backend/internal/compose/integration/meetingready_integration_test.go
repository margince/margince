// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "I'm ready" on both doors. An agent reads the preparation revision through
// prep_for_meeting and confirms it with no approval. The REST route and the
// tool answer alike, and a meeting moved in between refuses the old revision.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type preparationWire struct {
	ActivityID string   `json:"activity_id"`
	State      string   `json:"state"`
	Revision   string   `json:"revision"`
	Changed    []string `json:"changed"`
}

func (d *twoDoors) preparationOf(t *testing.T, meeting string) preparationWire {
	t.Helper()
	var prep struct {
		Preparation *preparationWire `json:"preparation"`
	}
	raw := d.tool(t, "prep_for_meeting", map[string]any{"record_type": "activity", "record_id": meeting})
	if err := json.Unmarshal(raw, &prep); err != nil || prep.Preparation == nil {
		t.Fatalf("prep_for_meeting names no preparation for the host's meeting (%v): %s", err, raw)
	}
	return *prep.Preparation
}

func decodePreparation(t *testing.T, raw json.RawMessage) preparationWire {
	t.Helper()
	var out preparationWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the answer is no preparation: %v\n%s", err, raw)
	}
	return out
}

func TestAnAgentConfirmsReadinessAtTheRevisionItReadOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "ready-doors", "read", "write")
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	meeting := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Pricing review", "body": "Agenda: the renewal terms",
		"occurred_at": start.Format(time.RFC3339), "meeting_status": "booked", "source": "manual",
	})

	read := d.preparationOf(t, meeting)
	if read.State != "needs_review" || read.Revision == "" {
		t.Fatalf("a meeting with an agenda reads %+v; an agenda is not readiness", read)
	}
	confirmed := decodePreparation(t, d.rest(t, "PUT", "/v1/worklist/ready",
		AnyMap{"activity_id": meeting, "preparation_revision": read.Revision}))
	if confirmed.State != "ready" || confirmed.Revision != read.Revision {
		t.Errorf("the route answered %+v, want ready at %s", confirmed, read.Revision)
	}
	assertTheAgentWrote(t, d.e, meeting, "update")

	if status := d.e.Call(t, "PATCH", "/v1/activities/"+meeting,
		AnyMap{"occurred_at": start.Add(time.Hour).Format(time.RFC3339)}, nil, nil); status != http.StatusOK {
		t.Fatalf("moving the meeting → %d", status)
	}
	moved := d.preparationOf(t, meeting)
	if moved.State != "needs_review" || len(moved.Changed) != 1 || moved.Changed[0] != "start_time" {
		t.Errorf("after the move the agent reads %+v, want needs review naming the start time", moved)
	}
	refusal := d.mcp.CallRefused(t, "confirm_meeting_ready", map[string]any{
		"activity_id": meeting, "preparation_revision": read.Revision,
	})
	if !strings.Contains(refusal, "skew") && !strings.Contains(refusal, "conflict") {
		t.Errorf("the stale revision was refused with %q, want a conflict", refusal)
	}
	again := decodePreparation(t, d.tool(t, "confirm_meeting_ready", map[string]any{
		"activity_id": meeting, "preparation_revision": moved.Revision,
	}))
	if again.State != "ready" {
		t.Errorf("confirming the current revision answered %+v", again)
	}

	undone := decodePreparation(t, d.rest(t, "POST", "/v1/worklist/ready/undo",
		AnyMap{"activity_id": meeting, "preparation_revision": moved.Revision}))
	if undone.State != "needs_review" {
		t.Errorf("the undo answered %+v, want needs review", undone)
	}
}
