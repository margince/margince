// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Words a model polished read AI-assisted on every read, the mark survives an
// edit, and a patch can set it but never lift it.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPolishedWordsReadAIAssistedAndThePatchNeverLiftsTheMark(t *testing.T) {
	s := setupStanding(t)
	ctx := s.As(s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms)
	link := s.contactAt(t, "Anna Berg", "anna@buyer.test")
	log := func(subject string, assisted bool) ids.UUID {
		t.Helper()
		logged, _, err := s.Activities.LogActivity(ctx, activities.LogActivityInput{
			Kind: "note", Subject: &subject, AIAssisted: assisted, AIAssistedOn: &link, Links: []activities.ActivityLinkInput{link},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.UUID(logged.Id)
	}
	assisted := func(row ids.UUID) bool {
		got := s.readTask(t, row).AiAssisted
		return got != nil && *got
	}
	polished, own := log("Call Anna about pricing", true), log("call anna re pricing", false)
	if !assisted(polished) || assisted(own) {
		t.Fatalf("ai_assisted polished=%t own=%t, want true and false", assisted(polished), assisted(own))
	}

	edit := "Call Anna about pricing on Friday"
	if _, err := s.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](polished),
		activities.UpdateActivityInput{Subject: &edit, AIAssisted: new(false)}); err != nil {
		t.Fatalf("editing the polished note: %v", err)
	}
	if !assisted(polished) {
		t.Error("an edit with ai_assisted false lifted the mark")
	}
	if _, err := s.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](own),
		activities.UpdateActivityInput{Subject: &edit, AIAssisted: new(true)}); err != nil {
		t.Fatalf("writing polished words onto the rep's note: %v", err)
	}
	if !assisted(own) {
		t.Error("a patch carrying polished words did not mark them")
	}
}

// Only a human marks words AI-assisted, and only on a record the activity is
// logged on: Polish wording asked about that record's restriction.
func TestOnlyAHumanMarksWordsAIAssistedOnTheRecordTheyArePolishedOn(t *testing.T) {
	d := newTwoDoors(t, "ai-assisted-mark", "read", "write")
	anna := AnyMap{"entity_type": "contact", "entity_id": createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Anna Berg"})}
	other := AnyMap{"entity_type": "contact", "entity_id": createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ben Roth"})}
	note := func(on AnyMap) AnyMap {
		return AnyMap{"kind": "note", "subject": "Call Anna about pricing", "ai_assisted": true, "ai_assisted_on": on, "links": []AnyMap{anna}}
	}
	refused := func(what string, status int, answer json.RawMessage, field string) {
		t.Helper()
		if status != http.StatusUnprocessableEntity || !strings.Contains(string(answer), `"field":"`+field+`"`) {
			t.Errorf("%s answered %d %s, want 422 naming %s", what, status, answer, field)
		}
	}
	var answer json.RawMessage
	refused("an agent marking its note", d.e.Call(t, "POST", "/v1/activities", note(anna), d.bearer, &answer), answer, "ai_assisted")
	refused("a mark polished on another record", d.e.Call(t, "POST", "/v1/activities", note(other), nil, &answer), answer, "ai_assisted_on")
	refused("a mark naming no record", d.e.Call(t, "POST", "/v1/activities",
		AnyMap{"kind": "note", "subject": "x", "ai_assisted": true, "links": []AnyMap{anna}}, nil, &answer), answer, "ai_assisted_on")

	var created struct {
		ID         string `json:"id"`
		AIAssisted *bool  `json:"ai_assisted"`
	}
	if status := d.e.Call(t, "POST", "/v1/activities", note(anna), nil, &created); status != http.StatusCreated || created.AIAssisted == nil || !*created.AIAssisted {
		t.Fatalf("a human's polished note answered %d, ai_assisted %v; want 201 and true", status, created.AIAssisted)
	}
	plain := createdRecord(t, d.e, "/v1/activities", AnyMap{"kind": "note", "subject": "call anna", "links": []AnyMap{anna}})
	refused("an agent marking a note it patches", d.e.Call(t, "PATCH", "/v1/activities/"+plain,
		AnyMap{"subject": "Call Anna", "ai_assisted": true}, d.bearer, &answer), answer, "ai_assisted")
}
