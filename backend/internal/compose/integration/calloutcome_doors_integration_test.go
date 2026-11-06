// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type outcomeRow struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	CallOutcome *string `json:"call_outcome"`
}

// A call records how it ended through either door: each of the five outcomes,
// and none. An outcome alone is a whole call. An outcome on a note and an
// unknown outcome are refused. The list filters calls by outcome.
func TestACallRecordsHowItEndedOverBothDoors(t *testing.T) {
	d := newTwoDoors(t, "call-outcome-doors", "read", "write")
	outcomes := []string{"reached", "not_reached", "voicemail", "call_back_later", "put_through"}
	log := func(t *testing.T, door string, fields map[string]any) json.RawMessage {
		t.Helper()
		if door == "http" {
			fields["source"] = "manual"
			return d.restStatus(t, "POST", "/v1/activities", fields, http.StatusCreated)
		}
		return d.tool(t, "create_record", map[string]any{"record_type": "activity", "fields": fields})
	}
	for _, door := range []string{"http", "tool"} {
		t.Run(door, func(t *testing.T) {
			for _, outcome := range outcomes {
				made := log(t, door, map[string]any{"kind": "call", "call_outcome": outcome})
				var row outcomeRow
				if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/activities/"+anIDIn(t, made), nil, http.StatusOK), &row); err != nil {
					t.Fatal(err)
				}
				if row.CallOutcome == nil || *row.CallOutcome != outcome {
					t.Errorf("a call logged %q reads %v", outcome, row.CallOutcome)
				}
			}
			plain := log(t, door, map[string]any{"kind": "call", "subject": "Quick check-in"})
			var row outcomeRow
			if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/activities/"+anIDIn(t, plain), nil, http.StatusOK), &row); err != nil {
				t.Fatal(err)
			}
			if row.CallOutcome != nil {
				t.Errorf("a call logged without an outcome reads %q", *row.CallOutcome)
			}
		})
	}

	// An outcome on a note names the field it was wrong on.
	refusedNote := d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "note", "subject": "Fair", "call_outcome": "reached", "source": "manual"}, http.StatusUnprocessableEntity)
	if !strings.Contains(string(refusedNote), "call_outcome") || !strings.Contains(string(refusedNote), "field_not_valid_for_kind") {
		t.Errorf("an outcome on a note was refused as %s, want field_not_valid_for_kind on call_outcome", refusedNote)
	}
	if refusal := d.toolRefused(t, "create_record", map[string]any{"record_type": "activity",
		"fields": map[string]any{"kind": "note", "subject": "Fair", "call_outcome": "reached"}}); !strings.Contains(refusal, "call_outcome") {
		t.Errorf("the tool refused an outcome on a note as %q, want it to name call_outcome", refusal)
	}
	if refused := d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "call", "call_outcome": "busy", "source": "manual"}, http.StatusUnprocessableEntity); !strings.Contains(string(refused), "call_outcome") {
		t.Errorf("an unknown outcome was refused as %s, want it to name call_outcome", refused)
	}
	if refusal := d.toolRefused(t, "create_record", map[string]any{"record_type": "activity",
		"fields": map[string]any{"kind": "call", "call_outcome": "busy"}}); !strings.Contains(refusal, "call_outcome") {
		t.Errorf("the tool refused an unknown outcome as %q, want it to name call_outcome", refusal)
	}

	// The list filters by outcome: one voicemail per door.
	raw := d.restStatus(t, "GET", "/v1/activities?kind=call&call_outcome=voicemail", nil, http.StatusOK)
	var list struct {
		Data []outcomeRow `json:"data"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 2 {
		t.Fatalf("filtered list = %s, want the two voicemail calls", raw)
	}
	for _, row := range list.Data {
		if row.CallOutcome == nil || *row.CallOutcome != "voicemail" {
			t.Errorf("the voicemail filter answered %+v", row)
		}
	}
	if refusal := d.toolRefused(t, "list_records", map[string]any{"record_type": "activity",
		"filters": map[string]any{"call_outcome": "voicmail"}}); !strings.Contains(refusal, "call_outcome") {
		t.Errorf("a misspelt outcome filter was refused as %q, want it to name call_outcome", refusal)
	}
}

// A patch sets and changes an outcome, with one audit row and one event per
// change; resending the stored outcome is no change; an outcome on a note is
// refused; and a call keeps standing on its outcome when its subject goes.
func TestAPatchSetsAndChangesACallOutcome(t *testing.T) {
	d := newTwoDoors(t, "call-outcome-patch", "read", "write")
	call := anIDIn(t, d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "call", "subject": "Ring the buyer", "source": "manual"}, http.StatusCreated))
	patch := func(id string, body map[string]any, status int) json.RawMessage {
		t.Helper()
		return d.restStatus(t, "PATCH", "/v1/activities/"+id, body, status)
	}
	patch(call, map[string]any{"call_outcome": "voicemail"}, http.StatusOK)
	patch(call, map[string]any{"call_outcome": "reached"}, http.StatusOK)
	patch(call, map[string]any{"call_outcome": "reached"}, http.StatusOK)
	audits := ownerCount(t, d, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND action='update' AND after ? 'call_outcome'`, call)
	events := ownerCount(t, d, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$1
		AND envelope->'payload'->'changed_fields' ? 'call_outcome'`, call)
	if audits != 2 || events != 2 {
		t.Errorf("two changes and a resend wrote %d audit rows and %d events, want 2 and 2", audits, events)
	}
	if n := ownerCount(t, d, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$1
		AND envelope->'payload'->'changed_fields'->>'call_outcome'='reached'`, call); n != 1 {
		t.Errorf("the event for the second change does not carry reached (%d)", n)
	}
	// The outcome holds the call up without its subject.
	patch(call, map[string]any{"subject": ""}, http.StatusOK)

	bare := anIDIn(t, d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "call", "subject": "No outcome yet", "source": "manual"}, http.StatusCreated))
	patch(bare, map[string]any{"subject": ""}, http.StatusUnprocessableEntity)

	// A wrong pick goes back to "no outcome" through an explicit null; a
	// patch that leaves the field out changes nothing.
	wrong := anIDIn(t, d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "call", "subject": "Ring the buyer", "call_outcome": "voicemail", "source": "manual"}, http.StatusCreated))
	patch(wrong, map[string]any{"subject": "Rang the buyer"}, http.StatusOK)
	var kept outcomeRow
	if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/activities/"+wrong, nil, http.StatusOK), &kept); err != nil {
		t.Fatal(err)
	}
	if kept.CallOutcome == nil || *kept.CallOutcome != "voicemail" {
		t.Fatalf("a patch that left call_outcome out changed it to %v", kept.CallOutcome)
	}
	patch(wrong, map[string]any{"call_outcome": nil}, http.StatusOK)
	var cleared outcomeRow
	if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/activities/"+wrong, nil, http.StatusOK), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.CallOutcome != nil {
		t.Errorf("an explicit null left the outcome %q", *cleared.CallOutcome)
	}
	if n := ownerCount(t, d, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND action='update'
		AND before->>'call_outcome' = 'voicemail' AND after->'call_outcome' = 'null'::jsonb`, wrong); n != 1 {
		t.Errorf("the clear wrote %d audit rows naming the old and the empty outcome, want 1", n)
	}
	if n := ownerCount(t, d, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$1
		AND envelope->'payload'->'changed_fields'->>'call_outcome_cleared'='true'`, wrong); n != 1 {
		t.Errorf("the clear raised %d call_outcome_cleared events, want 1", n)
	}

	note := anIDIn(t, d.restStatus(t, "POST", "/v1/activities",
		map[string]any{"kind": "note", "subject": "Fair notes", "source": "manual"}, http.StatusCreated))
	refused := patch(note, map[string]any{"call_outcome": "reached"}, http.StatusUnprocessableEntity)
	if !strings.Contains(string(refused), "call_outcome") {
		t.Errorf("an outcome patched onto a note was refused as %s", refused)
	}
}

// ownerCount reads one count past row security, as the auditor would.
func ownerCount(t *testing.T, d *twoDoors, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.e.Owner.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
