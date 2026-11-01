// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Home's worklist as the route serves it: from the reader's cached answer until
// the access fence or their own write ends it.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const homeWorklistPath = "/v1/worklist?scope=mine&filter=all&relationships=omit&view=today&limit=100"

func homeShows(t *testing.T, e *apptest.AppEnv, subject string) bool {
	t.Helper()
	var raw json.RawMessage
	if status := e.Call(t, "GET", homeWorklistPath, nil, nil, &raw); status != http.StatusOK {
		t.Fatalf("GET worklist → %d", status)
	}
	return strings.Contains(string(raw), subject)
}

// overdueTask writes a task for the reader straight to the table: a write no
// fence trigger and no request of theirs announces.
func overdueTask(t *testing.T, e *apptest.AppEnv, assignee ids.UUID, subject string) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO activity (kind, subject, due_at, is_done, assignee_id, source, captured_by)
		VALUES ('task', $1, now() - interval '1 hour', false, $2, 'manual', $3)`,
		subject, assignee, "human:"+assignee.String()); err != nil {
		t.Fatal(err)
	}
}

func TestHomeOpensOnItsCachedWorklistUntilTheFenceOrTheReadersWriteEndsIt(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Cache", "admin@homecache.test", "Admin")
	var admin ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = 'admin@homecache.test'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}

	homeShows(t, e, "warm")
	overdueTask(t, e, admin, "Call the quiet buyer")
	if homeShows(t, e, "Call the quiet buyer") {
		t.Fatal("the second read was assembled afresh; the cached answer was not served")
	}

	var contact struct {
		ID ids.UUID `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Fresh Lead"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("POST contact → %d", status)
	}
	if !homeShows(t, e, "Call the quiet buyer") {
		t.Error("the reader's own write left their Home on the answer from before it")
	}

	overdueTask(t, e, admin, "Send the revised offer")
	if homeShows(t, e, "Send the revised offer") {
		t.Fatal("the read after the refresh was not cached again")
	}
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET visibility = 'owner' WHERE id = $1`, contact.ID); err != nil {
		t.Fatal(err)
	}
	if !homeShows(t, e, "Send the revised offer") {
		t.Error("a change of reach left Home on the answer from before it")
	}
}

// The first page's cursor is minted under the question the service resolved.
// A request that leaves the scope to its default is still cached and served.
func TestACachedFirstPageKeepsTheWalkItsDefaultSpellingStarted(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Walk", "admin@homewalk.test", "Admin")
	var admin ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = 'admin@homewalk.test'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	overdueTask(t, e, admin, "First of two")
	overdueTask(t, e, admin, "Second of two")

	const onePage = "/v1/worklist?filter=all&relationships=omit&view=today&limit=1"
	var first, again struct {
		NextCursor *string `json:"next_cursor"`
	}
	if status := e.Call(t, "GET", onePage, nil, nil, &first); status != http.StatusOK || first.NextCursor == nil {
		t.Fatalf("first page → %d with cursor %v, want 200 and a walk to continue", status, first.NextCursor)
	}
	if status := e.Call(t, "GET", onePage, nil, nil, &again); status != http.StatusOK {
		t.Fatalf("the cached first page → %d", status)
	}
	if again.NextCursor == nil || *again.NextCursor != *first.NextCursor {
		t.Errorf("the second read did not serve the cached page and its walk")
	}
	var next json.RawMessage
	if status := e.Call(t, "GET", onePage+"&cursor="+*again.NextCursor, nil, nil, &next); status != http.StatusOK {
		t.Errorf("the cached page's cursor → %d, want the walk to continue", status)
	}
}
