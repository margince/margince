// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A read-only passport reaches no route-backed act, on either door. The census
// comes from the routes the build serves, so a new write action is held here.
func TestAReadOnlyPassportReachesNoRouteBackedWrite(t *testing.T) {
	d := newTwoDoors(t, "agent-read-only", "read")
	writes := 0
	for n, v := range compose.RouteVerbs() {
		if v.Scope == "read" {
			continue
		}
		if writes > 0 && writes%passportCallsPerBatch == 0 {
			bearer, _ := apptest.PassportWithID(t, d.e, fmt.Sprintf("agent-read-only %d", n), "read")
			d.bearer, d.mcp = bearer, apptest.NewMCPClient(d.e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))
		}
		writes++
		args, path := map[string]any{}, v.Pattern
		for _, param := range v.PathParams {
			args[param] = placeholderFor(param)
			path = strings.Replace(path, "{"+param+"}", placeholderFor(param), 1)
		}
		var body any
		if v.Body {
			args["request_body"], body = map[string]any{}, map[string]any{}
		}
		if v.UploadField != "" {
			args["filename"], args["content_type"], args["content_base64"] = "sweep.txt", "text/plain", "c3dlZXA="
		}
		// The router turns away a call missing a required parameter before the
		// gate, so 422 also means the route never ran.
		if status := d.e.Call(t, v.Method, path, body, d.bearer, nil); status != http.StatusForbidden && status != http.StatusUnprocessableEntity {
			t.Errorf("%s (%s %s) answers a read-only passport %d, want a refusal", v.Verb, v.Method, v.Pattern, status)
		}
		if got := d.mcp.Call(t, v.Verb, args); !got.IsError || !strings.Contains(got.Text, "scope") {
			t.Errorf("%s answers a read-only passport %+v, want a scope refusal", v.Verb, got)
		}
	}
	if writes == 0 {
		t.Fatal("the build serves no route-backed write, so this reached nothing")
	}
}

// colleagueDoors adds a rep to d's workspace and answers their agent's doors. The
// session stays with the admin.
func colleagueDoors(t *testing.T, d *twoDoors, adminEmail string) *twoDoors {
	t.Helper()
	const password = "correct-horse-battery"
	rep := ids.NewV7()
	email := "rep-" + rep.String()[24:] + "@colleague.test"
	seedInWorkspace(
		t, d.e, apptest.InstallationWorkspaceUUID(t.Context(), t, d.e.Owner),
		stmt(`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`, rep, email),
		stmt(`UPDATE app_user SET password_hash = (SELECT password_hash FROM app_user WHERE email = $1) WHERE id = $2`, adminEmail, rep),
		stmt(`INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'rep'`, rep),
	)
	login := func(who string) {
		t.Helper()
		if status := d.e.Call(t, "POST", "/v1/auth/login", AnyMap{"email": who, "password": password}, nil, nil); status != http.StatusOK {
			t.Fatalf("login as %s → %d", who, status)
		}
	}
	login(email)
	bearer, _ := apptest.PassportWithID(t, d.e, "colleague's agent", "read", "write", "send")
	login(adminEmail)
	return &twoDoors{e: d.e, bearer: bearer, mcp: apptest.NewMCPClient(d.e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))}
}

// A colleague's agent cannot reach another user's items. A rep's own grants turn
// some away, and the rest answer as a record that is not there.
func TestAColleaguesAgentReachesNoneOfAnotherUsersItems(t *testing.T) {
	const adminEmail = "agent-colleague@fable.test"
	d := newTwoDoors(t, "agent-colleague", "read", "write", "send")
	colleague := colleagueDoors(t, d, adminEmail)

	exclusion := answerID(t, captureAct(t, d, "add_exclusion", map[string]any{"request_body": AnyMap{"scope": "user", "kind": "domain", "value": "mine.test"}}))
	hold := answerID(t, captureAct(t, d, "add_counterparty_hold", map[string]any{"request_body": AnyMap{"kind": "domain", "value": "lawyers.test"}}))
	identity := answerID(t, captureAct(t, d, "add_owner_identity", map[string]any{"request_body": AnyMap{"kind": "address", "value": "me@private.test"}}))
	captureAct(t, d, "set_sender_decision", map[string]any{"address": "buyer@customer.test", "request_body": AnyMap{"decision": "business"}})
	view := answerID(t, d.tool(t, "change_settings", map[string]any{"action": "create_saved_view", "request_body": AnyMap{
		"resource": "contacts", "name": "Mine", "query": AnyMap{"columns": []any{"full_name"}},
	}}))
	selection := AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{firstReportMetric(t, d)}}
	report := decodeVersioned(t, reportAct(t, d, "create_report", map[string]any{"request_body": AnyMap{"name": "Private", "audience": "private", "selection": selection}}))
	share := answerID(t, []byte(reportAct(t, d, "create_forecast_share", map[string]any{"request_body": AnyMap{"kind": "live", "target": "forecast"}})))
	initiative := answerID(t, []byte(reportAct(t, d, "create_initiative", map[string]any{"request_body": AnyMap{
		"name": "Mine", "objective_metric": firstReportMetric(t, d), "deadline": clocktest.Now(t).AddDate(0, 3, 0).Format("2006-01-02"),
	}})))

	for name, call := range map[string]struct {
		tool string
		args map[string]any
	}{
		"lifting a user's exclusion":           {"change_capture", map[string]any{"action": "lift_exclusion", "id": exclusion}},
		"lifting a user's hold":                {"change_capture", map[string]any{"action": "lift_counterparty_hold", "id": hold}},
		"withdrawing a user's address":         {"change_capture", map[string]any{"action": "withdraw_owner_identity", "id": identity}},
		"withdrawing a user's sender":          {"change_capture", map[string]any{"action": "withdraw_sender_decision", "address": "buyer@customer.test"}},
		"archiving a user's view":              {"change_settings", map[string]any{"action": "archive_saved_view", "id": view}},
		"archiving a private report":           {"change_reports", map[string]any{"action": "archive_report", "id": report.ID}},
		"reading a private report's revisions": {"read_reports", map[string]any{"kind": "report_revisions", "id": report.ID}},
		"revoking a user's share":              {"change_reports", map[string]any{"action": "revoke_forecast_share", "id": share}},
		"launching a user's initiative":        {"change_reports", map[string]any{"action": "launch_initiative", "id": initiative, "if_version": 1}},
	} {
		got := colleague.mcp.Call(t, call.tool, call.args)
		refused := strings.Contains(got.Text, "No such record") || strings.Contains(got.Text, "permission denied")
		if !got.IsError || !refused {
			t.Errorf("%s answered a colleague's agent %+v, want a refusal", name, got)
		}
	}

	// The same acts work for the colleague on their own items, so the refusals
	// above came from whose item it is.
	own := answerID(t, captureAct(t, colleague, "add_exclusion", map[string]any{"request_body": AnyMap{"scope": "user", "kind": "domain", "value": "theirs.test"}}))
	captureAct(t, colleague, "lift_exclusion", map[string]any{"id": own})
	ownHold := answerID(t, captureAct(t, colleague, "add_counterparty_hold", map[string]any{"request_body": AnyMap{"kind": "domain", "value": "theirs.test"}}))
	captureAct(t, colleague, "lift_counterparty_hold", map[string]any{"id": ownHold})

	// The items are all still there for their user.
	if status := d.e.Call(t, "GET", "/v1/views/"+view, nil, nil, nil); status != http.StatusOK {
		t.Errorf("the user reading their own view → %d", status)
	}
	if status := d.e.Call(t, "GET", "/v1/analytics/reports/"+report.ID, nil, nil, nil); status != http.StatusOK {
		t.Errorf("the user reading their own report → %d", status)
	}
	holds := string(d.tool(t, "read_workspace", map[string]any{"topic": "counterparty_holds"}))
	if !strings.Contains(holds, "lawyers.test") {
		t.Errorf("the colleague's agent lifted a hold: %s", holds)
	}
}

// Consent is read under the contact's row scope. A contact private to its owner
// reads as not there for a colleague's agent, as it does for the colleague.
func TestAColleaguesAgentReadsNoConsentOnAPrivateContact(t *testing.T) {
	const adminEmail = "agent-consent-scope@fable.test"
	d := newTwoDoors(t, "agent-consent-scope", "read", "write", "send")
	colleague := colleagueDoors(t, d, adminEmail)
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Private Subject", "owner_id": memberID(t, d, adminEmail),
		"emails": []AnyMap{{"email": "private@subject.test", "email_type": "work", "is_primary": true}},
	})
	if _, err := d.e.Owner.Exec(t.Context(), `UPDATE contact SET visibility = 'owner' WHERE id = $1`, contact); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	d.tool(t, "read_details", map[string]any{"action": "contact_consent", "id": contact})
	for _, action := range []string{"contact_consent", "contact_consent_guard"} {
		got := colleague.mcp.Call(t, "read_details", map[string]any{"action": action, "id": contact})
		if !got.IsError || !strings.Contains(got.Text, "No such record") {
			t.Errorf("%s of a private contact answered a colleague's agent %+v, want not found", action, got)
		}
	}
}
