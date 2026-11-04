// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// complianceDoors is a workspace with one contact and one consent purpose, and a
// passport whose user is the workspace's admin.
type complianceDoors struct {
	*twoDoors
	contact string
	purpose string
}

func newComplianceDoors(t *testing.T, slug string) complianceDoors {
	t.Helper()
	d := newTwoDoors(t, slug, "read", "write", "send")
	var purposes struct {
		Data []struct {
			ID                  string `json:"id"`
			RequiresDoubleOptIn bool   `json:"requires_double_opt_in"`
		} `json:"data"`
	}
	if status := d.e.Call(t, "GET", "/v1/consent-purposes", nil, nil, &purposes); status != http.StatusOK || len(purposes.Data) == 0 {
		t.Fatalf("reading the purposes → %d, %d purposes", status, len(purposes.Data))
	}
	purpose := purposes.Data[0].ID
	for _, p := range purposes.Data {
		if !p.RequiresDoubleOptIn {
			purpose = p.ID
			break
		}
	}
	return complianceDoors{
		twoDoors: d, purpose: purpose,
		contact: createdRecord(t, d.e, "/v1/contacts", AnyMap{
			"source": "manual", "full_name": "Dana Subject",
			"emails": []AnyMap{{"email": "dana@subject.test", "email_type": "work", "is_primary": true}},
		}),
	}
}

func (c complianceDoors) act(t *testing.T, action string, args map[string]any) json.RawMessage {
	t.Helper()
	args["action"] = action
	return c.tool(t, "change_compliance", args)
}

func isAgentStamp(stamp string) bool { return strings.HasPrefix(stamp, "agent:") }

// A consent an agent records names the agent on the proof row. Its source says
// so too, whatever source the call claims.
func TestAnAgentsConsentProofNamesTheAgent(t *testing.T) {
	c := newComplianceDoors(t, "agent-consent-proof")
	c.act(t, "record_consent", map[string]any{"id": c.contact, "request_body": AnyMap{
		"purpose_id": c.purpose, "new_state": "granted", "lawful_basis": "consent",
		"source": "web_form", "wording": "You agree we may write to you.",
	}})
	var source, capturedBy, liveSource string
	if err := c.e.Owner.QueryRow(t.Context(), `
		SELECT e.source, e.captured_by, s.source
		  FROM consent_event e JOIN contact_consent s USING (contact_id, purpose_id)
		 WHERE e.contact_id = $1 AND e.purpose_id = $2`,
		c.contact, c.purpose).Scan(&source, &capturedBy, &liveSource); err != nil {
		t.Fatalf("reading the proof row: %v", err)
	}
	if source != "agent" || liveSource != "agent" || !isAgentStamp(capturedBy) {
		t.Errorf("proof source=%q state source=%q captured_by=%q, want the agent named on each", source, liveSource, capturedBy)
	}
}

// A user's own consent keeps saying what the app has always said it was.
func TestAUsersOwnConsentProofIsNotAnAgents(t *testing.T) {
	c := newComplianceDoors(t, "user-consent-proof")
	if status := c.e.Call(t, "POST", "/v1/contacts/"+c.contact+"/consent", AnyMap{
		"purpose_id": c.purpose, "new_state": "granted", "lawful_basis": "consent",
		"source": "web_form", "wording": "You agree we may write to you.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("recording consent in the app → %d", status)
	}
	var source, capturedBy string
	if err := c.e.Owner.QueryRow(t.Context(),
		`SELECT source, captured_by FROM consent_event WHERE contact_id = $1 AND purpose_id = $2`,
		c.contact, c.purpose).Scan(&source, &capturedBy); err != nil {
		t.Fatalf("reading the proof row: %v", err)
	}
	if source != "web_form" || !strings.HasPrefix(capturedBy, "human:") {
		t.Errorf("source=%q captured_by=%q, want the app's own form and the user", source, capturedBy)
	}
}

// The stop and the vouch run on a real contact. Each row names the agent, and
// each can be taken back through its own action.
func TestAnAgentRecordsAndTakesBackAStopAndAVouch(t *testing.T) {
	c := newComplianceDoors(t, "agent-stops")
	c.act(t, "suppress_contact", map[string]any{"id": c.contact, "request_body": AnyMap{
		"kind": "subject_request", "reason": "Asked on the phone.",
	}})
	var stop, stopBy string
	if err := c.e.Owner.QueryRow(t.Context(),
		`SELECT id, captured_by FROM communication_suppression WHERE contact_id = $1`, c.contact).Scan(&stop, &stopBy); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if !isAgentStamp(stopBy) {
		t.Errorf("the stop's captured_by = %q, want the agent", stopBy)
	}
	// An admin never outranks an admin's stop, so the stop reads as a rep's, the
	// one level this admin's agent may lift.
	if _, err := c.e.Owner.Exec(t.Context(),
		`UPDATE communication_suppression SET decided_by_level = 'user' WHERE id = $1`, stop); err != nil {
		t.Fatalf("lowering the stop: %v", err)
	}
	c.act(t, "lift_suppression", map[string]any{"id": c.contact, "suppressionId": stop, "request_body": AnyMap{
		"reason": "They wrote back and asked us to carry on.",
	}})

	var vouch struct {
		ID string `json:"override_id"`
	}
	raw := c.act(t, "allow_contact", map[string]any{"id": c.contact, "request_body": AnyMap{
		"category": "marketing", "reason": "They asked for the newsletter at the fair.",
	}})
	if err := json.Unmarshal(raw, &vouch); err != nil || vouch.ID == "" {
		t.Fatalf("allow_contact answered %s (%v), want the override id", raw, err)
	}
	var vouchBy string
	if err := c.e.Owner.QueryRow(t.Context(),
		`SELECT captured_by FROM communication_override WHERE id = $1`, vouch.ID).Scan(&vouchBy); err != nil {
		t.Fatalf("reading the vouch: %v", err)
	}
	if !isAgentStamp(vouchBy) {
		t.Errorf("the vouch's captured_by = %q, want the agent", vouchBy)
	}
	c.act(t, "revoke_override", map[string]any{"id": c.contact, "overrideId": vouch.ID, "request_body": AnyMap{
		"reason": "The fair was another contact.",
	}})

	var stopLive, vouchLive bool
	if err := c.e.Owner.QueryRow(t.Context(), `
		SELECT (SELECT revoked_at IS NULL FROM communication_suppression WHERE id = $1),
		       (SELECT revoked_at IS NULL FROM communication_override WHERE id = $2)`, stop, vouch.ID).Scan(&stopLive, &vouchLive); err != nil {
		t.Fatalf("reading the reversals: %v", err)
	}
	if stopLive || vouchLive {
		t.Errorf("after the reversals the stop is live=%v and the vouch is live=%v, want neither", stopLive, vouchLive)
	}
}

// A hand-recorded exchange is legal evidence, so its row says an agent wrote it.
func TestAnAgentsQualifyingEventNamesTheAgent(t *testing.T) {
	c := newComplianceDoors(t, "agent-qualifying")
	event := answerID(t, c.act(t, "record_qualifying_event", map[string]any{"id": c.contact, "request_body": AnyMap{
		"kind": "in_person", "note": "Met at the Hannover fair.",
		"occurred_at": clocktest.Now(t).Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	}}))
	defer func() {
		c.act(t, "retract_qualifying_event", map[string]any{"id": c.contact, "event_id": event})
		var retracted bool
		if err := c.e.Owner.QueryRow(t.Context(),
			`SELECT retracted_at IS NOT NULL FROM consent_qualifying_event WHERE id = $1`, event).Scan(&retracted); err != nil || !retracted {
			t.Errorf("the agent's retraction left retracted=%v (%v)", retracted, err)
		}
		acquired := c.mcp.Call(t, "change_compliance", map[string]any{"action": "retract_acquisition_request", "id": c.contact})
		if acquired.IsError && strings.Contains(acquired.Text, "permission denied") {
			t.Errorf("retracting the acquisition request refused the user's agent: %s", acquired.Text)
		}
	}()
	var source, capturedBy string
	if err := c.e.Owner.QueryRow(t.Context(),
		`SELECT source, captured_by FROM consent_qualifying_event WHERE contact_id = $1`, c.contact).Scan(&source, &capturedBy); err != nil {
		t.Fatalf("reading the exchange: %v", err)
	}
	if source != "agent" || !isAgentStamp(capturedBy) {
		t.Errorf("source=%q captured_by=%q, want the agent", source, capturedBy)
	}
}

// Mail to a contact runs on send_outbound and spends send. This stack has no mail
// relay and its purpose takes no double opt-in. So each call passes every check on
// the agent and ends on the precondition named here.
func TestAnAgentMailsTheComplianceMessages(t *testing.T) {
	c := newComplianceDoors(t, "agent-compliance-mail")
	const noRelay = "cannot send its own mail yet"
	for _, call := range []struct {
		args   map[string]any
		reason string
	}{
		{map[string]any{"as": "privacy_notice", "id": c.contact}, noRelay},
		{map[string]any{"as": "details_confirmation", "id": c.contact}, noRelay},
		{map[string]any{"as": "double_opt_in", "id": c.contact, "request_body": AnyMap{"purpose_id": c.purpose}}, "not confirmed by double opt-in"},
	} {
		got := c.mcp.Call(t, "send_outbound", call.args)
		if !got.IsError || !strings.Contains(got.Text, call.reason) {
			t.Errorf("send_outbound %v answered %+v, want it to end on %q", call.args["as"], got, call.reason)
		}
	}
	readOnly := newTwoDoors(t, "agent-compliance-noscope", "read", "write")
	got := readOnly.mcp.Call(t, "send_outbound", map[string]any{"as": "privacy_notice", "id": c.contact})
	if !got.IsError || !strings.Contains(got.Text, "scope") {
		t.Errorf("a passport without send answered %+v, want a scope refusal", got)
	}
}

// An agent checks a contact before it records, and reads what the user reads.
func TestAnAgentReadsAContactsConsentAsTheUserDoes(t *testing.T) {
	c := newComplianceDoors(t, "agent-consent-read")
	read := func(action string) string {
		t.Helper()
		return string(c.tool(t, "read_details", map[string]any{"action": action, "id": c.contact}))
	}
	if before := read("contact_consent"); strings.Contains(before, c.purpose) && strings.Contains(before, `"granted"`) {
		t.Fatalf("the fixture's contact already granted the purpose: %s", before)
	}
	c.act(t, "record_consent", map[string]any{"id": c.contact, "request_body": AnyMap{
		"purpose_id": c.purpose, "new_state": "granted", "lawful_basis": "consent",
		"source": "web_form", "wording": "You agree we may write to you.",
	}})
	for action, route := range map[string]string{
		"contact_consent":       "/v1/contacts/" + c.contact + "/consent",
		"contact_consent_guard": "/v1/contacts/" + c.contact + "/consent/guard",
	} {
		var asUser json.RawMessage
		if status := c.e.Call(t, "GET", route, nil, nil, &asUser); status != http.StatusOK {
			t.Fatalf("the user's own read of %s → %d", route, status)
		}
		var want, got any
		if err := json.Unmarshal(asUser, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(read(action)), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s answers the agent %v and the user %v", action, got, want)
		}
	}
	if after := read("contact_consent"); !strings.Contains(after, `"granted"`) || !strings.Contains(after, `"actor_type":"agent"`) {
		t.Errorf("the read does not show the consent the agent recorded, or who recorded it: %s", after)
	}
	missing := c.mcp.Call(t, "read_details", map[string]any{"action": "contact_consent", "id": "00000000-0000-4000-8000-000000000001"})
	if !missing.IsError || !strings.Contains(missing.Text, "not found") {
		t.Errorf("a contact that does not exist answered %+v, want not found", missing)
	}
}

// The subject-request queue and its deadline, read and worked as the user does.
func TestAnAgentWorksTheSubjectRequestQueue(t *testing.T) {
	c := newComplianceDoors(t, "agent-dsr")
	var opened struct {
		ID string `json:"id"`
	}
	raw := c.act(t, "open_data_subject_request", map[string]any{"request_body": AnyMap{
		"kind": "access", "subject_ref": "dana@subject.test",
	}})
	if err := json.Unmarshal(raw, &opened); err != nil || opened.ID == "" {
		t.Fatalf("opening a request answered %s (%v)", raw, err)
	}
	statusInQueue := func() string {
		t.Helper()
		var queue struct {
			Data []struct {
				ID     string `json:"id"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
			} `json:"data"`
		}
		raw := c.tool(t, "read_workspace", map[string]any{"topic": "data_subject_requests"})
		if err := json.Unmarshal(raw, &queue); err != nil || len(queue.Data) != 1 ||
			queue.Data[0].ID != opened.ID || queue.Data[0].Kind != "access" {
			t.Fatalf("the queue answered %s (%v), want the one access request the agent opened", raw, err)
		}
		return queue.Data[0].Status
	}
	if got := statusInQueue(); got != "open" {
		t.Errorf("the new request reads %q, want open", got)
	}
	received := clocktest.Now(t).UTC()
	var deadline struct {
		DueAt time.Time `json:"due_at"`
		Rule  string    `json:"rule"`
	}
	raw = c.tool(t, "read_workspace", map[string]any{
		"topic": "data_subject_request_deadline", "kind": "access", "received_at": received.Format(time.RFC3339),
	})
	if err := json.Unmarshal(raw, &deadline); err != nil || !deadline.DueAt.After(received) || deadline.Rule == "" {
		t.Errorf("the deadline answered %s (%v), want a rule and a day after the request came in", raw, err)
	}
	c.act(t, "update_data_subject_request", map[string]any{"id": opened.ID, "request_body": AnyMap{"status": "in_progress"}})
	if got := statusInQueue(); got != "in_progress" {
		t.Errorf("the worked request reads %q, want in_progress", got)
	}
}

// An erase cannot be taken back, so an agent works the request and a human closes it.
func TestAnAgentDoesNotCloseAnErasure(t *testing.T) {
	c := newComplianceDoors(t, "agent-no-erase")
	var opened struct {
		ID string `json:"id"`
	}
	raw := c.act(t, "open_data_subject_request", map[string]any{"request_body": AnyMap{
		"kind": "erasure", "subject_ref": c.contact,
	}})
	if err := json.Unmarshal(raw, &opened); err != nil || opened.ID == "" {
		t.Fatalf("opening the request answered %s (%v)", raw, err)
	}
	got := c.mcp.Call(t, "change_compliance", map[string]any{
		"action": "update_data_subject_request", "id": opened.ID,
		"request_body": AnyMap{"status": "fulfilled", "resolution": "Erased as asked."},
	})
	if !got.IsError || !strings.Contains(got.Text, "permission denied") {
		t.Errorf("an agent fulfilling an erasure answered %+v, want a permission refusal", got)
	}
	if status := c.e.Call(t, "GET", "/v1/contacts/"+c.contact, nil, nil, nil); status != http.StatusOK {
		t.Errorf("the contact after the refused erasure → %d", status)
	}
}

// The compliance reads answer the agent what they answer its user.
func TestAnAgentReadsTheComplianceQueues(t *testing.T) {
	c := newComplianceDoors(t, "agent-compliance-reads")
	for _, read := range []struct {
		route string
		topic string
	}{
		{"/v1/audit-log", "audit_log"},
		{"/v1/retention/legal-holds", "legal_holds"},
		{"/v1/retention/restrictions", "restricted_records"},
		{"/v1/privacy/notice-cases", "notice_cases"},
		{"/v1/privacy/notice-case-reviews", "notice_case_reviews"},
	} {
		if status := c.e.Call(t, "GET", read.route, nil, c.bearer, nil); status != http.StatusOK {
			t.Errorf("GET %s as a passport → %d", read.route, status)
		}
		var asUser json.RawMessage
		if status := c.e.Call(t, "GET", read.route, nil, nil, &asUser); status != http.StatusOK {
			t.Fatalf("the user's own read of %s → %d", read.route, status)
		}
		var want, got any
		if err := json.Unmarshal(asUser, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(c.tool(t, "read_workspace", map[string]any{"topic": read.topic}), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s answers the agent %v and the user %v", read.topic, got, want)
		}
	}
	history := c.tool(t, "read_workspace", map[string]any{
		"topic": "record_history", "entity_type": "contact", "id": c.contact,
	})
	if !strings.Contains(string(history), "create") {
		t.Errorf("a contact's history %s does not hold its creation", history)
	}
}

// Placing a hold is open and lifting it is not. Pinning is open, its evidence
// names the agent, and the release that erases stays with the user.
func TestAnAgentPlacesAHoldAndPinsARecordButNeverLiftsOrReleases(t *testing.T) {
	c := newComplianceDoors(t, "agent-holds")
	reason := AnyMap{"reason": "Litigation on this account."}
	c.act(t, "place_legal_hold", map[string]any{"entityType": "contact", "recordId": c.contact, "request_body": reason})
	var held bool
	if err := c.e.Owner.QueryRow(t.Context(), `SELECT legal_hold FROM contact WHERE id = $1`, c.contact).Scan(&held); err != nil {
		t.Fatalf("reading the hold: %v", err)
	}
	if !held {
		t.Error("place_legal_hold left the contact unheld")
	}
	if status := c.e.Call(t, "POST", "/v1/retention/legal-holds/contact/"+c.contact+"/lift", reason, c.bearer, nil); status != http.StatusForbidden {
		t.Errorf("an agent lifting a hold → %d, want 403", status)
	}

	var note struct {
		ID string `json:"id"`
	}
	if status := c.e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "note", "subject": "A record worth keeping", "source": "manual",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": c.contact}},
	}, nil, &note); status != http.StatusCreated {
		t.Fatalf("logging a note → %d", status)
	}
	c.act(t, "pin_activity_to_floor", map[string]any{"activityId": note.ID, "request_body": reason})
	var decidedBy string
	if err := c.e.Owner.QueryRow(t.Context(),
		`SELECT decided_by_name FROM activity_retention_evidence WHERE activity_id = $1`, note.ID).Scan(&decidedBy); err != nil {
		t.Fatalf("reading the pin's evidence: %v", err)
	}
	if !strings.HasPrefix(decidedBy, "Agent for ") {
		t.Errorf("the pin's evidence names %q, want the agent named for its user", decidedBy)
	}
	if status := c.e.Call(t, "POST", "/v1/retention/restrictions/"+note.ID+"/release", reason, c.bearer, nil); status != http.StatusForbidden {
		t.Errorf("an agent releasing a pinned record → %d, want 403", status)
	}
}

// The acts security keeps with the user are refused on the route and have no
// action on any router.
func TestAnAgentIsRefusedTheComplianceActsSecurityKeeps(t *testing.T) {
	c := newComplianceDoors(t, "agent-compliance-closed")
	for _, closed := range []struct{ method, route string }{
		{"PATCH", "/v1/retention/settings"},
		{"POST", "/v1/retention-policies"},
		{"POST", "/v1/consent-purposes"},
		{"PUT", "/v1/privacy/controller-particulars"},
		{"POST", "/v1/privacy/notice-cases/00000000-0000-4000-8000-000000000001/excuse"},
		{"PATCH", "/v1/capture/settings"},
		{"POST", "/v1/capture/exclusions/00000000-0000-4000-8000-000000000001/purge"},
		{"DELETE", "/v1/knowledge/documents/00000000-0000-4000-8000-000000000001"},
		{"POST", "/v1/exports"},
		{"POST", "/v1/admin/reporting/pause"},
		{"PUT", "/v1/forecast/assurance/enforcement"},
	} {
		if status := c.e.Call(t, closed.method, closed.route, AnyMap{}, c.bearer, nil); status != http.StatusForbidden {
			t.Errorf("an agent on %s %s → %d, want 403", closed.method, closed.route, status)
		}
	}
}
