// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"
)

func TestCombinedFollowUpOverBothDoors(t *testing.T) {
	d := newTwoDoors(t, "combined-follow-up-doors", "read", "write")
	for _, door := range []string{"http", "tool"} {
		t.Run(door, func(t *testing.T) {
			fields := map[string]any{"kind": "note", "subject": "Scope review", "follow_up_task": map[string]any{"subject": "Send revised scope"}}
			var made json.RawMessage
			if door == "http" {
				fields["source"] = "manual"
				made = d.restStatus(t, "POST", "/v1/activities", fields, http.StatusCreated)
			} else {
				made = d.tool(t, "log_activity", fields)
			}
			source := anIDIn(t, made)
			raw := d.restStatus(t, "GET", "/v1/activities?source_activity_id="+source, nil, http.StatusOK)
			var list struct {
				Data []sharedActivity `json:"data"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				t.Fatal(err)
			}
			if len(list.Data) != 1 {
				t.Fatalf("follow-ups = %s", raw)
			}
			task := list.Data[0]
			if task.Kind != "task" || task.Subject == nil || *task.Subject != "Send revised scope" || task.SourceActivityID == nil || *task.SourceActivityID != source || task.DueAt != nil {
				t.Fatalf("follow-up lost its work, relation or undated state: %+v", task)
			}
		})
	}
}

func TestCombinedFollowUpHTTPReplayPreservesOneTask(t *testing.T) {
	d := newTwoDoors(t, "combined-follow-up-replay", "read", "write")
	fields := map[string]any{
		"kind": "note", "subject": "Scope review", "source": "manual",
		"follow_up_task": map[string]any{"subject": "Send revised scope"},
	}
	headers := maps.Clone(d.bearer)
	headers["Idempotency-Key"] = "scope-review-follow-up"
	first := d.restStatusWith(t, "POST", "/v1/activities", fields, headers, http.StatusCreated)
	replay := d.restStatusWith(t, "POST", "/v1/activities", fields, headers, http.StatusCreated)
	if anIDIn(t, first) != anIDIn(t, replay) {
		t.Fatalf("replay changed the source: %s vs %s", first, replay)
	}
	fields["follow_up_task"] = map[string]any{"subject": "Send a different scope"}
	d.restStatusWith(t, "POST", "/v1/activities", fields, headers, http.StatusConflict)
	raw := d.restStatus(t, "GET", "/v1/activities?source_activity_id="+anIDIn(t, first), nil, http.StatusOK)
	var list struct {
		Data []sharedActivity `json:"data"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].Subject == nil || *list.Data[0].Subject != "Send revised scope" {
		t.Fatalf("replay duplicated or changed the task: %s", raw)
	}
}
