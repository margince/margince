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

func captureAct(t *testing.T, d *twoDoors, action string, args map[string]any) json.RawMessage {
	t.Helper()
	args["action"] = action
	return d.tool(t, "change_capture", args)
}

func answerID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.ID == "" {
		t.Fatalf("the answer %s names no id (%v)", raw, err)
	}
	return got.ID
}

// A rule or hold the user made stays theirs, so their agent lifts only what an agent
// added. The user adding it again makes the decision theirs.
func TestAnAgentLiftsNoCaptureRuleOrHoldTheUserDecided(t *testing.T) {
	d := newTwoDoors(t, "agent-capture-decided", "read", "write")
	for _, list := range []struct {
		path, add, lift string
		body            AnyMap
	}{
		{"/v1/capture/exclusions", "add_exclusion", "lift_exclusion", AnyMap{"scope": "user", "kind": "domain", "value": "kept.test"}},
		{"/v1/capture/counterparty-holds", "add_counterparty_hold", "lift_counterparty_hold", AnyMap{"kind": "domain", "value": "kept.test"}},
	} {
		t.Run(list.add, func(t *testing.T) {
			agentAdded := answerID(t, captureAct(t, d, list.add, map[string]any{"request_body": list.body}))
			var added struct {
				ID string `json:"id"`
			}
			if status := d.e.Call(t, "POST", list.path, list.body, nil, &added); status >= http.StatusBadRequest || added.ID != agentAdded {
				t.Fatalf("the user adding it again → %d, id %s, want %s", status, added.ID, agentAdded)
			}
			refusal := d.toolRefused(t, "change_capture", map[string]any{"action": list.lift, "id": agentAdded})
			if !strings.Contains(refusal, "the user already decided") {
				t.Errorf("the agent lifting the user's decision answered %q", refusal)
			}
			var problem struct {
				Detail string `json:"detail"`
			}
			status := d.e.Call(t, "DELETE", list.path+"/"+agentAdded, nil, d.bearer, &problem)
			if status != http.StatusConflict || !strings.HasPrefix(problem.Detail, "the user already decided about this ") || !strings.HasSuffix(problem.Detail, "so the user changes it") {
				t.Errorf("the route answered %d %q, want 409 and the plain sentence alone", status, problem.Detail)
			}
			if status := d.e.Call(t, "DELETE", list.path+"/"+agentAdded, nil, nil, nil); status != http.StatusNoContent {
				t.Errorf("the user lifting it → %d", status)
			}
		})
	}
}

// The user's own capture lists, written and read back through the agent.
func TestAnAgentKeepsTheUsersCaptureLists(t *testing.T) {
	d := newTwoDoors(t, "agent-capture-lists", "read", "write")
	rule := answerID(t, captureAct(t, d, "add_exclusion", map[string]any{"request_body": AnyMap{
		"scope": "user", "kind": "domain", "value": "newsletter.test",
	}}))
	captureAct(t, d, "lift_exclusion", map[string]any{"id": rule})

	hold := answerID(t, captureAct(t, d, "add_counterparty_hold", map[string]any{"request_body": AnyMap{
		"kind": "domain", "value": "lawyers.test",
	}}))
	holds := d.tool(t, "read_workspace", map[string]any{"topic": "counterparty_holds"})
	if !strings.Contains(string(holds), "lawyers.test") {
		t.Errorf("the holds %s do not hold what the agent added", holds)
	}
	captureAct(t, d, "lift_counterparty_hold", map[string]any{"id": hold})

	address := answerID(t, captureAct(t, d, "add_owner_identity", map[string]any{"request_body": AnyMap{
		"kind": "address", "value": "me@private.test",
	}}))
	mine := d.tool(t, "read_workspace", map[string]any{"topic": "owner_identities"})
	if !strings.Contains(string(mine), "me@private.test") {
		t.Errorf("the identities %s do not hold what the agent declared", mine)
	}
	captureAct(t, d, "withdraw_owner_identity", map[string]any{"id": address})
	captureAct(t, d, "restore_owner_identity", map[string]any{"id": address})

	for _, topic := range []string{"held_threads", "capture_senders", "contacts_awaiting_decision"} {
		d.tool(t, "read_workspace", map[string]any{"topic": topic})
	}
}

// An agent may say a sender is business. keep_out destroys mail for good, so it
// stays with the user, who may still do it in the app.
func TestAnAgentDecidesASenderButNeverDestroysItsMail(t *testing.T) {
	d := newTwoDoors(t, "agent-capture-sender", "read", "write")
	captureAct(t, d, "set_sender_decision", map[string]any{
		"address": "buyer@customer.test", "request_body": AnyMap{"decision": "business"},
	})
	captureAct(t, d, "withdraw_sender_decision", map[string]any{"address": "buyer@customer.test"})

	got := d.mcp.Call(t, "change_capture", map[string]any{
		"action": "set_sender_decision", "address": "spam@sender.test", "request_body": AnyMap{"decision": "keep_out"},
	})
	if !got.IsError || !strings.Contains(got.Text, "permission denied") {
		t.Errorf("an agent's keep_out answered %+v, want a refusal", got)
	}
	if status := d.e.Call(t, "PUT", "/v1/capture/senders/spam@sender.test/decision",
		AnyMap{"decision": "keep_out"}, nil, nil); status != http.StatusOK {
		t.Errorf("the user's own keep_out → %d", status)
	}
}

// An agent's decision about a domain says so. A user's decision stands over it,
// and a machine verdict stands under it.
func TestAnAgentsDomainDecisionNamesItAndNeverReplacesAUsers(t *testing.T) {
	d := newTwoDoors(t, "agent-capture-domain", "read", "write")
	block := func(domain string) map[string]any {
		return map[string]any{"request_body": AnyMap{"domain": domain, "admission": "suppressed", "reason": "A competitor's newsletter."}}
	}
	captureAct(t, d, "set_blocked_domain", block("agentmade.test"))
	sourceOf := func(domain string) string {
		t.Helper()
		var source string
		if err := d.e.Owner.QueryRow(t.Context(),
			`SELECT admission_source FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&source); err != nil {
			t.Fatalf("reading %s: %v", domain, err)
		}
		return source
	}
	if got := sourceOf("agentmade.test"); got != "agent" {
		t.Errorf("an agent's decision reads as %q, want agent", got)
	}

	if status := d.e.Call(t, "PUT", "/v1/capture/blocked-domains", AnyMap{
		"domain": "usermade.test", "admission": "admitted", "reason": "They became a client.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("the user's own decision → %d", status)
	}
	got := d.mcp.Call(t, "change_capture", map[string]any{
		"action": "set_blocked_domain", "request_body": AnyMap{"domain": "usermade.test", "admission": "suppressed", "reason": "Looks like a newsletter."},
	})
	if !got.IsError {
		t.Errorf("an agent replaced a user's decision: %+v", got)
	}
	if source := sourceOf("usermade.test"); source != "human" {
		t.Errorf("the user's decision now reads as %q", source)
	}
	if status := d.e.Call(t, "PUT", "/v1/capture/blocked-domains", AnyMap{
		"domain": "agentmade.test", "admission": "admitted", "reason": "They became a client after all.",
	}, nil, nil); status != http.StatusOK {
		t.Errorf("the user overruling their agent → %d", status)
	}
}

// Corpora are defined and archived through the agent. Asking one needs enrich.
func TestAnAgentDefinesACorpusAndAskingOneSpendsEnrich(t *testing.T) {
	d := newTwoDoors(t, "agent-corpus", "read", "write")
	corpus := answerID(t, captureAct(t, d, "create_corpus", map[string]any{"request_body": AnyMap{
		"name": "Field notes", "topic_statement": "What the field team learned.",
	}}))
	captureAct(t, d, "update_corpus", map[string]any{"id": corpus, "request_body": AnyMap{"topic_statement": "Notes from the field."}})
	got := d.mcp.Call(t, "run_research", map[string]any{"action": "ask_corpus", "id": corpus, "request_body": AnyMap{"question": "What did we learn?"}})
	if !got.IsError || !strings.Contains(got.Text, "scope") {
		t.Errorf("asking a corpus without enrich answered %+v, want a scope refusal", got)
	}
	captureAct(t, d, "archive_corpus", map[string]any{"id": corpus})
}

// senderStamp reads who made a seat's decision about a sender.
func senderStamp(t *testing.T, d *twoDoors, address string) (decision string, capturedBy *string) {
	t.Helper()
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT decision, captured_by FROM capture_sender_override WHERE address = $1`, address).Scan(&decision, &capturedBy); err != nil {
		t.Fatalf("reading the decision about %s: %v", address, err)
	}
	return decision, capturedBy
}

// A decision the user made about a sender stays theirs. Their agent can neither
// replace nor withdraw it, and may do both to its own.
func TestAnAgentNeverReplacesOrWithdrawsAUsersSenderDecision(t *testing.T) {
	d := newTwoDoors(t, "agent-sender-typed", "read", "write")
	const mine, theirs = "buyer@agent.test", "family@user.test"

	captureAct(t, d, "set_sender_decision", map[string]any{"address": mine, "request_body": AnyMap{"decision": "business"}})
	if _, by := senderStamp(t, d, mine); by == nil || !isAgentStamp(*by) {
		t.Errorf("the agent's decision names %v, want the agent", by)
	}

	if status := d.e.Call(t, "PUT", "/v1/capture/senders/"+theirs+"/decision", AnyMap{"decision": "keep_out"}, nil, nil); status != http.StatusOK {
		t.Fatalf("the user's own decision → %d", status)
	}
	for name, args := range map[string]map[string]any{
		"replacing":   {"action": "set_sender_decision", "address": theirs, "request_body": AnyMap{"decision": "business"}},
		"withdrawing": {"action": "withdraw_sender_decision", "address": theirs},
	} {
		if got := d.mcp.Call(t, "change_capture", args); !got.IsError || !strings.Contains(got.Text, "user already decided") {
			t.Errorf("%s the user's decision answered %+v, want a refusal that says the user decided", name, got)
		}
	}
	if decision, by := senderStamp(t, d, theirs); decision != "keep_out" || by == nil || isAgentStamp(*by) {
		t.Errorf("the user's decision reads %q by %v after the agent's attempts", decision, by)
	}

	if status := d.e.Call(t, "PUT", "/v1/capture/senders/"+mine+"/decision", AnyMap{"decision": "keep_out"}, nil, nil); status != http.StatusOK {
		t.Errorf("the user overruling their agent → %d", status)
	}
	if _, by := senderStamp(t, d, mine); by == nil || isAgentStamp(*by) {
		t.Errorf("the user's overruling decision still names %v", by)
	}
	captureAct(t, d, "set_sender_decision", map[string]any{"address": "new@agent.test", "request_body": AnyMap{"decision": "business"}})
	captureAct(t, d, "withdraw_sender_decision", map[string]any{"address": "new@agent.test"})
}

// The workspace's domain lists, and what a hold already caught.
func TestAnAgentKeepsTheWorkspaceDomainListsOnRealRows(t *testing.T) {
	d := newTwoDoors(t, "agent-domain-lists", "read", "write")
	consumer := answerID(t, captureAct(t, d, "add_consumer_mail_domain", map[string]any{"request_body": AnyMap{"domain": "post.agent.test", "kind": "extra"}}))
	captureAct(t, d, "remove_consumer_mail_domain", map[string]any{"id": consumer})
	captureAct(t, d, "add_workspace_email_domain", map[string]any{"request_body": AnyMap{"domain": "own.agent.test"}})
	captureAct(t, d, "remove_workspace_email_domain", map[string]any{"domain": "own.agent.test"})
	captureAct(t, d, "add_counterparty_hold", map[string]any{"request_body": AnyMap{"kind": "domain", "value": "hold.agent.test"}})
	captureAct(t, d, "share_counterparty_hold_history", map[string]any{})
}

// An agent agreeing with a user's domain decision has what it asked for. Going
// against one is refused, and either way the user's decision stands.
func TestAnAgentAgreesWithAUsersDomainDecisionButCannotReverseIt(t *testing.T) {
	d := newTwoDoors(t, "agent-domain-sticky", "read", "write")
	const domain = "stickyagent.test"
	if status := d.e.Call(t, "PUT", "/v1/capture/blocked-domains", AnyMap{
		"domain": domain, "admission": "admitted", "reason": "They became a client.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("the user's decision → %d", status)
	}
	captureAct(t, d, "set_blocked_domain", map[string]any{"request_body": AnyMap{
		"domain": domain, "admission": "admitted", "reason": "Keep it.",
	}})
	got := d.mcp.Call(t, "change_capture", map[string]any{"action": "set_blocked_domain", "request_body": AnyMap{
		"domain": domain, "admission": "suppressed", "reason": "Looks like a newsletter.",
	}})
	if !got.IsError || !strings.Contains(got.Text, "already decided") {
		t.Errorf("an agent reversing the user answered %+v, want a refusal naming the decision", got)
	}
	var source string
	if err := d.e.Owner.QueryRow(t.Context(),
		`SELECT admission_source FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&source); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if source != "human" {
		t.Errorf("the decision now reads source %q, want the user's", source)
	}
}

// Reading a colleague's work and handing a project on, as the owner does.
func TestAnAgentCorrectsAHealthReadingAndHandsOnAProject(t *testing.T) {
	d := newTwoDoors(t, "agent-project-work", "read", "write")
	d.e.DescribeCompany(t)
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Project Works"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Rollout", "company_id": company, "source": "manual"})
	assessment := answerID(t, d.tool(t, "change_state", map[string]any{
		"action": "record_project_health", "id": project, "request_body": AnyMap{"state": "at_risk", "note": "Slipped."},
	}))
	d.tool(t, "change_state", map[string]any{
		"action": "correct_project_health", "id": project, "assessment_id": assessment,
		"request_body": AnyMap{"state": "on_track", "note": "Back on plan."},
	})
	var source, capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT source, captured_by FROM project_health_assessment WHERE project_id = $1 ORDER BY id DESC LIMIT 1`, project).Scan(&source, &capturedBy); err != nil {
		t.Fatalf("reading the correction: %v", err)
	}
	if source != "agent" || !isAgentStamp(capturedBy) {
		t.Errorf("the correction reads source=%q captured_by=%q, want the agent", source, capturedBy)
	}

	self := memberID(t, d, "agent-project-work@fable.test")
	colleague := createdID(t, d.e, "/v1/users", AnyMap{"email": "rep@agent-project-work.test", "display_name": "Rep", "role": "rep"})
	if _, err := d.e.Owner.Exec(t.Context(), `UPDATE app_user SET status = 'active', password_hash = 'x' WHERE id = $1`, colleague); err != nil {
		t.Fatalf("activating the colleague: %v", err)
	}
	if status := d.e.Call(t, "PATCH", "/v1/projects/"+project, AnyMap{"owner_id": self}, nil, nil); status != http.StatusOK {
		t.Fatalf("giving the project an owner → %d", status)
	}
	d.tool(t, "change_state", map[string]any{"action": "transfer_projects", "request_body": AnyMap{"from_owner_id": self, "to_owner_id": colleague}})
	var owner string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT owner_id::text FROM project WHERE id = $1`, project).Scan(&owner); err != nil {
		t.Fatalf("reading the owner: %v", err)
	}
	if owner != colleague {
		t.Errorf("the project is owned by %s, want %s", owner, colleague)
	}
}
