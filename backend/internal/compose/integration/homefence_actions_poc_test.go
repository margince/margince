// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && poc

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// How far each everyday action moves the Home fence. A step ends every
// member's kept Home answer, so this is what bounds the cache's hit rate.
func TestHowEverydayActionsMoveTheHomeFencePoC(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Fence PoC", "fence@poc.test", "Admin")
	stages := apptest.DiscoverSeededPipeline(t, e)
	fence := func() int64 {
		var n int64
		if err := e.Pool.QueryRow(context.Background(), `SELECT generation FROM home_read_fence`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	type created struct {
		ID string `json:"id"`
	}
	var contact, task, deal, note created
	call := func(method, path string, body any, out any, want int) {
		t.Helper()
		if status := e.Call(t, method, path, body, nil, out); status != want {
			t.Fatalf("%s %s → %d, want %d", method, path, status, want)
		}
	}
	actions := []struct {
		name string
		run  func()
	}{
		{"create a contact with an email", func() {
			call("POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Fence Person",
				"emails": []AnyMap{{"email": "person@fence.test"}}}, &contact, http.StatusCreated)
		}},
		{"create a contact without an email", func() {
			call("POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Plain Person"}, nil, http.StatusCreated)
		}},
		{"log a note on the contact", func() {
			call("POST", "/v1/activities", AnyMap{"kind": "note", "body": "Called back",
				"links": []AnyMap{{"entity_type": "contact", "entity_id": contact.ID}}}, &note, http.StatusCreated)
		}},
		{"create a task", func() {
			call("POST", "/v1/activities", AnyMap{"kind": "task", "subject": "Send offer",
				"links": []AnyMap{{"entity_type": "contact", "entity_id": contact.ID}}}, &task, http.StatusCreated)
		}},
		{"rename the task", func() {
			call("PATCH", "/v1/activities/"+task.ID, AnyMap{"subject": "Send the offer"}, nil, http.StatusOK)
		}},
		{"complete the task", func() {
			call("PATCH", "/v1/activities/"+task.ID, AnyMap{"is_done": true}, nil, http.StatusOK)
		}},
		{"create a company", func() {
			call("POST", "/v1/companies", AnyMap{"display_name": "Fence GmbH", "source": "manual"}, nil, http.StatusCreated)
		}},
		{"create a deal", func() {
			call("POST", "/v1/deals", AnyMap{"name": "Fence deal", "pipeline_id": stages.PipelineID,
				"stage_id": stages.Open, "source": "manual"}, &deal, http.StatusCreated)
		}},
		{"file the note on the deal too", func() {
			call("POST", "/v1/activities/"+note.ID+"/relink", AnyMap{
				"entity_type": "deal", "entity_id": deal.ID}, nil, http.StatusOK)
		}},
		{"archive the note", func() {
			call("DELETE", "/v1/activities/"+note.ID, nil, nil, http.StatusOK)
		}},
		{"read Home", func() {
			call("GET", "/v1/worklist?scope=mine&filter=all&limit=100", nil, nil, http.StatusOK)
		}},
	}
	for _, action := range actions {
		before := fence()
		action.run()
		t.Logf("fence +%d  %s", fence()-before, action.name)
	}
}

// Capture's per-mail write: each captured mail is linked to its sender, and
// the sender is named on the mail's participants. Nothing here narrows reach.
func TestHowCapturingMailMovesTheHomeFencePoC(t *testing.T) {
	e := Setup(t)
	at := time.Now().UTC().Add(-time.Hour)
	for i, label := range []string{"first mail from a new sender", "second mail from the same sender", "third mail from the same sender"} {
		mail := seedMailFrom(t, "buyer@fence-capture.test", at.Add(time.Duration(i)*time.Minute))
		before := homeFence(t, e)
		captureSender(t, e, mail, "buyer@fence-capture.test", "fence-capture.test")
		t.Logf("fence +%d  capture: %s", homeFence(t, e)-before, label)
	}
}

// Which trigger the capture step moves the fence through: each run turns one
// fence trigger off in this test's own database and captures one more mail.
func TestWhichTriggerCaptureMovesTheHomeFenceThroughPoC(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	at := time.Now().UTC().Add(-time.Hour)
	for i, trigger := range []string{"", "activity_participant home_read_fence_participant", "activity_link home_read_fence_first_link"} {
		if trigger != "" {
			table, name, _ := strings.Cut(trigger, " ")
			if _, err := owner.Exec(context.Background(), "ALTER TABLE "+table+" DISABLE TRIGGER "+name); err != nil {
				t.Fatal(err)
			}
		}
		mail := seedMailFrom(t, "buyer@fence-which.test", at.Add(time.Duration(i)*time.Minute))
		before := homeFence(t, e)
		captureSender(t, e, mail, "buyer@fence-which.test", "fence-which.test")
		t.Logf("fence +%d  with %q also off", homeFence(t, e)-before, trigger)
	}
}
