// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The contract's Idempotency-Key promise, end to end: a keyed POST
// replayed with the identical body returns the ORIGINAL status and body
// and creates exactly one domain row; the same key with a different
// body is refused (409 idempotency_key_conflict, per the parameter's
// contract description) instead of silently replaying mismatched
// intent; and the transport key takes precedence over the natural
// (source_system, source_id) dedupe on logActivity — the replay never
// reaches the store.

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestIdempotencyKeyReplay(t *testing.T) {
	e := apptest.SetupApp(t)

	apptest.BootstrapWorkspaceSession(t, e, "Idem Probe", "admin@idem.test", "Admin")

	keyed := map[string]string{"Idempotency-Key": "lead-retry-1"}
	leadReq := AnyMap{
		"full_name":    "Retry Prospect",
		"email":        "retry@example.org",
		"company_name": "Retry AG",
		"source":       "import:idem",
	}

	var first AnyMap
	if status := e.Call(t, "POST", "/v1/leads", leadReq, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed create lead = %d %v", status, first)
	}

	// The replay is byte-identical: same status, same body — NOT the 409
	// the natural email dedupe would answer if the request re-executed.
	var replay AnyMap
	if status := e.Call(t, "POST", "/v1/leads", leadReq, keyed, &replay); status != http.StatusCreated {
		t.Fatalf("keyed replay = %d %v, want the original 201", status, replay)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Errorf("replayed response differs from the original:\n first: %v\nreplay: %v", first, replay)
	}

	// Exactly one lead landed.
	var leads struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/leads", nil, nil, &leads); status != http.StatusOK {
		t.Fatalf("list leads = %d", status)
	}
	if len(leads.Data) != 1 {
		t.Fatalf("replayed create produced %d leads, want exactly 1", len(leads.Data))
	}

	// The same key with a DIFFERENT body is a conflict, never a replay.
	var problem AnyMap
	status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": "Different Intent",
		"email":     "other@example.org",
		"source":    "import:idem",
	}, keyed, &problem)
	if status != http.StatusConflict || problem["code"] != "idempotency_key_conflict" {
		t.Fatalf("mismatched body under a reused key = %d %v, want 409 idempotency_key_conflict", status, problem)
	}
}

// A replay is a read, and the recorded body is a frozen copy of the record.
// Once the record is gone the copy must go with it: Art. 17 erasure anonymizes
// the row in place, stamps archived_at and leaves owner_id alone, so a probe
// that only asks "is this still yours" answers yes for a tombstone and the
// middleware hands back the pre-erasure snapshot every live read path now
// refuses. The replay probe is live-only for exactly that reason.
func TestIdempotencyReplayRefusesAnArchivedRecord(t *testing.T) {
	e := apptest.SetupApp(t)

	apptest.BootstrapWorkspaceSession(t, e, "Idem Erase", "admin@idemerase.test", "Admin")

	keyed := map[string]string{"Idempotency-Key": "lead-erase-1"}
	leadReq := AnyMap{
		"full_name":    "Erasable Prospect",
		"email":        "erasable@example.org",
		"company_name": "Erasable AG",
		"source":       "import:idem",
	}

	var first AnyMap
	if status := e.Call(t, "POST", "/v1/leads", leadReq, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed create lead = %d %v", status, first)
	}
	leadID, _ := first["id"].(string)
	if leadID == "" {
		t.Fatalf("created lead carries no id: %v", first)
	}

	// The positive control, BEFORE the record goes: the replay works, so a
	// later refusal is the archive doing it and not the mechanism being broken.
	var live AnyMap
	if status := e.Call(t, "POST", "/v1/leads", leadReq, keyed, &live); status != http.StatusCreated {
		t.Fatalf("replay while the record is live = %d %v, want the original 201", status, live)
	}

	if status := e.Call(t, "DELETE", "/v1/leads/"+leadID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archiving the lead = %d, want 200", status)
	}

	var problem AnyMap
	status := e.Call(t, "POST", "/v1/leads", leadReq, keyed, &problem)
	if status != http.StatusNotFound {
		t.Fatalf("replay after the record was archived = %d %v, want 404 — the recorded body is a snapshot of a record that no longer exists",
			status, problem)
	}
	if problem["full_name"] != nil || problem["email"] != nil {
		t.Errorf("the refused replay leaked the recorded record body: %v", problem)
	}
}

// TestAPromoteReplayStillWorksThoughItArchivedItsOwnLead is the boundary the
// companion probe has to respect.
//
// Promoting a lead ARCHIVES it, and the response names that lead. A companion
// probe that asked whether the lead is still LIVE would answer no on every
// promote — refusing the retry the idempotency key exists to serve, which is a
// worse failure than the disclosure it was meant to close. What a companion
// discloses is an id, so what it is checked against is row SCOPE.
func TestAPromoteReplayStillWorksThoughItArchivedItsOwnLead(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Idem Promote", "admin@idempromote.test", "Admin")

	var lead AnyMap
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name":    "Promotable Prospect",
		"email":        "promotable@example.org",
		"company_name": "Promotable AG",
		"source":       "import:idem",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead = %d %v", status, lead)
	}
	leadID, _ := lead["id"].(string)
	if leadID == "" {
		t.Fatalf("created lead carries no id: %v", lead)
	}

	keyed := map[string]string{"Idempotency-Key": "promote-retry-1"}
	var first AnyMap
	promoteReq := AnyMap{"trigger": "human_qualify"}
	if status := e.Call(t, "POST", "/v1/leads/"+leadID+"/promote", promoteReq, keyed, &first); status != http.StatusOK {
		t.Fatalf("promote = %d %v", status, first)
	}
	if first["lead_id"] == nil {
		t.Fatalf("the promotion names no lead, so this test would prove nothing about the companion: %v", first)
	}

	// The lead is archived BY the promotion, so this is the retry a live probe
	// would have refused.
	var replay AnyMap
	if status := e.Call(t, "POST", "/v1/leads/"+leadID+"/promote", promoteReq, keyed, &replay); status != http.StatusOK {
		t.Fatalf("promote replay = %d %v, want the original 200 — the lead it archived is not a lost companion", status, replay)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Errorf("replayed response differs from the original:\n first: %v\nreplay: %v", first, replay)
	}
}

// TestIdempotencyKeyReplay_createQuota proves the promise for an
// operation with no natural-key dedupe behind it: without transport
// idempotency a retried createQuota lands a second, identical target.
func TestIdempotencyKeyReplay_createQuota(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var me AnyMap
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("/me = %d", status)
	}
	adminID := me["user"].(AnyMap)["id"].(string)

	keyed := map[string]string{"Idempotency-Key": "quota-retry-1"}
	quotaReq := AnyMap{
		"owner_id": adminID, "period_start": "2026-01-01", "period_end": "2026-03-31",
		"target_minor": 1000000, "currency": "EUR",
	}

	var first AnyMap
	if status := e.Call(t, "POST", "/v1/quotas", quotaReq, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed create quota = %d %v", status, first)
	}
	var replay AnyMap
	if status := e.Call(t, "POST", "/v1/quotas", quotaReq, keyed, &replay); status != http.StatusCreated {
		t.Fatalf("keyed replay = %d %v, want the original 201", status, replay)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Errorf("replayed response differs from the original:\n first: %v\nreplay: %v", first, replay)
	}

	var quotas struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/quotas", nil, nil, &quotas); status != http.StatusOK {
		t.Fatalf("list quotas = %d", status)
	}
	if len(quotas.Data) != 1 {
		t.Fatalf("replayed create produced %d quotas, want exactly 1", len(quotas.Data))
	}

	// The same key with a DIFFERENT body is a conflict, never a replay.
	var problem AnyMap
	status := e.Call(t, "POST", "/v1/quotas", AnyMap{
		"owner_id": adminID, "period_start": "2026-04-01", "period_end": "2026-06-30",
		"target_minor": 2000000, "currency": "EUR",
	}, keyed, &problem)
	if status != http.StatusConflict || problem["code"] != "idempotency_key_conflict" {
		t.Fatalf("mismatched body under a reused key = %d %v, want 409 idempotency_key_conflict", status, problem)
	}
}

func TestIdempotencyKeyReplay_logActivity(t *testing.T) {
	e := apptest.SetupApp(t)

	apptest.BootstrapWorkspaceSession(t, e, "Idem Activity", "admin@idem-act.test", "Admin")

	var person AnyMap
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Idem Person", "source": "ui",
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person = %d %v", status, person)
	}

	keyed := map[string]string{"Idempotency-Key": "act-retry-1"}
	logReq := AnyMap{
		"kind":    "note",
		"subject": "Keyed note",
		"source":  "ui",
		"links":   []AnyMap{{"entity_type": "person", "entity_id": person["id"]}},
	}

	var first, replay AnyMap
	if status := e.Call(t, "POST", "/v1/activities", logReq, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed log activity = %d %v", status, first)
	}
	if status := e.Call(t, "POST", "/v1/activities", logReq, keyed, &replay); status != http.StatusCreated {
		t.Fatalf("keyed activity replay = %d %v, want the original 201", status, replay)
	}
	if first["id"] != replay["id"] {
		t.Errorf("replay returned a different activity: %v vs %v", first["id"], replay["id"])
	}

	var activities struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/activities", nil, nil, &activities); status != http.StatusOK {
		t.Fatalf("list activities = %d", status)
	}
	if len(activities.Data) != 1 {
		t.Fatalf("replayed log produced %d activities, want exactly 1", len(activities.Data))
	}
}
