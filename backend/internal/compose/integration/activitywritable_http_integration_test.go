// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The contact 360 timeline and the single read are two readers of one fact,
// so they must give the same writable for the same activity.
func TestTheContact360TimelineSaysTheSameWritableAsTheSingleRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Dana Buyer"})
	activity := createdRecord(t, e, "/v1/activities", AnyMap{
		"kind": "note", "subject": "Call notes",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})

	var single struct {
		Writable *bool `json:"writable"`
	}
	if status := e.Call(t, "GET", "/v1/activities/"+activity, nil, nil, &single); status != http.StatusOK {
		t.Fatalf("GET activity = %d, want 200", status)
	}
	var view struct {
		Activities struct {
			Data []struct {
				ID       string `json:"id"`
				Writable *bool  `json:"writable"`
			} `json:"data"`
		} `json:"activities"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+contact+"/360", nil, nil, &view); status != http.StatusOK {
		t.Fatalf("GET 360 = %d, want 200", status)
	}
	if single.Writable == nil || !*single.Writable {
		t.Fatalf("single read writable = %v, want true for the creator", single.Writable)
	}
	for _, row := range view.Activities.Data {
		if row.ID != activity {
			continue
		}
		if row.Writable == nil || *row.Writable != *single.Writable {
			t.Fatalf("360 timeline writable = %v, single read = %v, want the same", row.Writable, *single.Writable)
		}
		return
	}
	t.Fatalf("activity %s missing from the 360 timeline", activity)
}
