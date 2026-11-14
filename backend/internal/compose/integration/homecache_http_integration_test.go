// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Home's worklist as the route serves it: from the reader's cached answer until
// the access fence or their own write ends it.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// Past its freshness the kept day is still served, and says so, so the browser
// asks again for the one read behind it.
func TestHomeServesItsKeptDayPastItsLifeAndSaysItIsStale(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Stale", "admin@homestale.test", "Admin")
	var admin ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = 'admin@homestale.test'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	homeShows(t, e, "warm")
	overdueTask(t, e, admin, "Chase the signed order")
	// The reader's earlier writes are aged with the answer, so it stays newer than them.
	for _, aged := range []string{
		`UPDATE home_read_cache SET computed_at = computed_at - $1::interval WHERE reader_id = $2`,
		`UPDATE home_read_forgotten SET at = at - $1::interval WHERE reader_id = $2`,
	} {
		if _, err := e.Owner.Exec(context.Background(), aged, (homecache.Life + time.Minute).String(), admin); err != nil {
			t.Fatal(err)
		}
	}
	var day struct {
		ServedStale *bool `json:"served_stale"`
	}
	var raw json.RawMessage
	if status := e.Call(t, "GET", homeWorklistPath, nil, nil, &raw); status != http.StatusOK {
		t.Fatalf("GET worklist → %d", status)
	}
	if err := json.Unmarshal(raw, &day); err != nil {
		t.Fatal(err)
	}
	if day.ServedStale == nil || !*day.ServedStale || strings.Contains(string(raw), "Chase the signed order") {
		t.Errorf("past its life Home answered served_stale=%v with the new task shown %v; want the kept day, flagged stale",
			day.ServedStale, strings.Contains(string(raw), "Chase the signed order"))
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
		t.Fatalf("the second read did not serve the cached page and its walk")
	}
	var next json.RawMessage
	if status := e.Call(t, "GET", onePage+"&cursor="+*again.NextCursor, nil, nil, &next); status != http.StatusOK {
		t.Errorf("the cached page's cursor → %d, want the walk to continue", status)
	}
}

func receiptComputedAt(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var raw json.RawMessage
	if status := e.Call(t, "GET", "/v1/magic?limit=100", nil, nil, &raw); status != http.StatusOK {
		t.Fatalf("GET magic → %d", status)
	}
	var at string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT computed_at::text FROM home_read_cache WHERE read = 'magic'`).Scan(&at); err != nil {
		t.Fatalf("reading the kept receipt: %v", err)
	}
	return at
}

func TestHomesReceiptIsServedFromCacheWhileAMachineWrites(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Receipt", "admin@homereceipt.test", "Admin")

	first := receiptComputedAt(t, e)
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('agent', 'agent:enrich', 'update', 'deal', gen_random_uuid())`); err != nil {
		t.Fatal(err)
	}
	if again := receiptComputedAt(t, e); again != first {
		t.Errorf("the receipt was computed again after a machine write (%s → %s), want the cached answer", first, again)
	}
}

// worklistAnswer reads Home's worklist with the reader's session, sending
// ifNoneMatch when it is set.
func worklistAnswer(t *testing.T, e *apptest.AppEnv, ifNoneMatch string) (status int, header http.Header, body []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.TS.URL+homeWorklistPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func TestAnUnchangedWorklistComesBackAs304AndNoSharedCacheKeepsIt(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Tag", "admin@hometag.test", "Admin")
	var admin ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = 'admin@hometag.test'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}

	status, header, body := worklistAnswer(t, e, "")
	etag := header.Get("ETag")
	if status != http.StatusOK || etag == "" || len(body) == 0 {
		t.Fatalf("the first read answered %d with tag %q and %d bytes, want 200, a tag and the day", status, etag, len(body))
	}
	if got := header.Get("Cache-Control"); got != "private, no-cache" {
		t.Errorf("the day carries Cache-Control %q, want private, no-cache", got)
	}
	status, header, body = worklistAnswer(t, e, etag)
	if status != http.StatusNotModified || len(body) != 0 || header.Get("Cache-Control") != "private, no-cache" {
		t.Errorf("an unchanged re-read answered %d with %d bytes and Cache-Control %q, want 304, none and private, no-cache",
			status, len(body), header.Get("Cache-Control"))
	}

	overdueTask(t, e, admin, "Chase the signed order")
	if err := homecache.New(e.Pool).Forget(
		principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool)),
		[]ids.UUID{admin}); err != nil {
		t.Fatal(err)
	}
	if status, header, body = worklistAnswer(t, e, etag); status != http.StatusOK || header.Get("ETag") == etag ||
		!strings.Contains(string(body), "Chase the signed order") {
		t.Errorf("a changed day answered %d with tag %q, want 200, a new tag and the new task", status, header.Get("ETag"))
	}
}
