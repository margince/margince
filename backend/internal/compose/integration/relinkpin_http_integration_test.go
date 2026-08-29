// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The version a relink was admitted on, re-checked by the write that moves it.
//
// A dynamic tier is resolved from a READ, and that read commits before the write
// it admits. An agent controls both sides of the window, so the verdict is only
// true of the record as it WAS — auth/admit.go binds the version for exactly
// this reason, and until #2614 nothing consumed it: RelinkActivityInput carried
// no version and the batch write compared none, on either door.
//
// The pin arrives here as If-Match, which is how the REST gate forwards it
// (compose/agentgate.go) and how a human's client states the version it read.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestARelinkIsRefusedWhenTheActivityMovedUnderIt(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Relink pin", "pin@fable.test", "Admin")
	personID, taskID := seedTaskAndTarget(t, e)

	var read struct {
		Version int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/activities/"+taskID, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading the activity → %d, want 200", status)
	}
	if read.Version == 0 {
		t.Fatal("the activity reports no version, so there is nothing for a pin to mean")
	}

	// The control: the version it was read at still moves it, so this case
	// cannot pass by the pin refusing everything.
	if status := e.Call(t, "POST", "/v1/activities/"+taskID+"/relink",
		AnyMap{"entity_type": "person", "entity_id": personID},
		map[string]string{"If-Match": strconv.FormatInt(read.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("relink on the version it was read at → %d, want 200", status)
	}

	// Something else moves the activity — which is what an agent's own window
	// looks like from the outside — and the stale pin loses to the compare
	// rather than to timing.
	if status := e.Call(t, "PATCH", "/v1/activities/"+taskID,
		AnyMap{"subject": "moved under the relink"}, nil, nil); status != http.StatusOK {
		t.Fatalf("moving the activity → %d, want 200", status)
	}
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", "/v1/activities/"+taskID+"/relink",
		AnyMap{"entity_type": "person", "entity_id": personID, "replace_existing_of_type": true},
		map[string]string{"If-Match": strconv.FormatInt(read.Version, 10)}, &problem); status != http.StatusConflict ||
		problem.Code != "version_skew" {
		t.Fatalf("relink on a stale version → %d (%s), want 409 version_skew — the pin the gate bound is what "+
			"stops a relink running on a verdict about the record as it was", status, problem.Code)
	}
}

// A thread and a named set move MANY activities, and one version cannot speak
// for them. Applied per row a pin would refuse every activity except whichever
// happened to match, which reads as a partial move nobody asked for — so the
// batch doors say so rather than dropping it silently.
func TestABatchRelinkRefusesAVersionPinRatherThanIgnoringIt(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Batch pin", "batch@fable.test", "Admin")
	personID, taskID := seedTaskAndTarget(t, e)

	var read struct {
		Version int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/activities/"+taskID, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading the activity → %d, want 200", status)
	}
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", "/v1/activities/relink-bulk",
		AnyMap{"activity_ids": []string{taskID}, "entity_type": "person", "entity_id": personID},
		map[string]string{"If-Match": strconv.FormatInt(read.Version, 10)}, &problem); status != http.StatusUnprocessableEntity ||
		problem.Code != "pin_not_supported" {
		t.Fatalf("a pinned batch relink → %d (%s), want 422 pin_not_supported — a version that cannot "+
			"condition the move must be refused by NAME, so a client can branch on the reason and an "+
			"unrelated 422 cannot pass for this one", status, problem.Code)
	}
}
