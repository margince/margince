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
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

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

// TestAScheduledSendReplayProbesTheScheduleNotTheActivity is the alt-table half
// of the replay probe.
//
// One route answers two tables behind the same "id": the outbound ACTIVITY when
// the message went now, the SCHEDULED SEND when the caller asked for later, and
// scheduled_at is what tells them apart. Probing the activity table for a
// scheduled send would look up an id that is not there and refuse every retry.
func TestAScheduledSendReplayProbesTheScheduleNotTheActivity(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	route := "/v1/activities/" + p.activityID + "/send-email"
	keyed := map[string]string{"Idempotency-Key": "schedule-retry-1"}
	req := AnyMap{
		"subject": "Monday morning", "body": "Written the night before.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}

	var first AnyMap
	if status := p.Call(t, "POST", route, req, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed schedule = %d %v", status, first)
	}
	if first["scheduled_at"] == nil {
		t.Fatalf("the answer carries no scheduled_at, so the replay would be gated on the activity table and this test would prove nothing: %v", first)
	}

	var replay AnyMap
	if status := p.Call(t, "POST", route, req, keyed, &replay); status != http.StatusCreated {
		t.Fatalf("keyed replay = %d %v, want the original 201", status, replay)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Errorf("replayed response differs from the original:\n first: %v\nreplay: %v", first, replay)
	}
}

// TestAScheduledSendReplayRefusesAMessageThatIsNoLongerTheCallersOwn is the
// refusal that probe exists for.
//
// A scheduled message is the SENDER's own: an unsent body and its blind-copy
// list are not workspace-readable the way a sent activity is, so the scope
// asked here is the scheduled_by predicate the store itself reads with. The
// row moving to another sender is what the test reaches through SQL — the
// transport key is scoped per principal, so nothing in the API can hand one
// caller another's key today, and this is the guard for the day something can.
func TestAScheduledSendReplayRefusesAMessageThatIsNoLongerTheCallersOwn(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	route := "/v1/activities/" + p.activityID + "/send-email"
	keyed := map[string]string{"Idempotency-Key": "schedule-retry-2"}
	req := AnyMap{
		"subject": "Monday morning", "body": "Written the night before.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}

	var first AnyMap
	if status := p.Call(t, "POST", route, req, keyed, &first); status != http.StatusCreated {
		t.Fatalf("keyed schedule = %d %v", status, first)
	}

	colleague := SeedIDRow(t, p.Owner,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'other@preflight.test', 'Other Sender')`)
	if _, err := p.Owner.Exec(context.Background(),
		`UPDATE scheduled_send SET scheduled_by = $1 WHERE id = $2`, colleague, first["id"]); err != nil {
		t.Fatalf("handing the schedule to another sender: %v", err)
	}

	// Existence-hiding, like every other row scope: somebody else's message is
	// not found rather than forbidden.
	var replay AnyMap
	if status := p.Call(t, "POST", route, req, keyed, &replay); status != http.StatusNotFound {
		t.Fatalf("replay of a schedule now owned by another sender = %d %v, want 404", status, replay)
	}
}
