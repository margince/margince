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

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/consent"
)

// A user's agent carries out the user's decision to send a refused message. The
// row names the user who took it on and the agent that acted. The refusal stays a
// refusal, and the user's own decision names no agent.
func TestAnAgentDirectsARefusedSendAndTheRowNamesItAndItsUser(t *testing.T) {
	d := newTwoDoorsWith(t, "agent-direct-send", []compose.Option{compose.WithOperatorMail(apptest.DiscardingMailer{})}, "read", "write")
	c := seedConsentSubject(t, d.e)
	c.useReviewableMarketingProfile(t)
	sendBearer, passport := apptest.PassportWithID(t, d.e, "direct-send agent", "read", "write", "send")
	agent := apptest.NewMCPClient(d.e, strings.TrimPrefix(sendBearer["Authorization"], "Bearer "))

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("correspondence with no supporting context → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	shown := string(agent.CallOK(t, "read_details", map[string]any{"action": "communication_review", "id": review}).Envelope(t).Data)
	if !strings.Contains(shown, consent.OverrideWarningVersion) {
		t.Fatalf("the review shows the agent no warning to put to its user: %s", shown)
	}

	refused := d.mcp.Call(t, "send_outbound", map[string]any{"as": "direct_send", "id": review, "request_body": directed()})
	if !refused.IsError || !strings.Contains(refused.Text, "scope") {
		t.Errorf("a passport without send answered %+v, want a scope refusal", refused)
	}
	if status := d.e.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), d.bearer, nil); status != http.StatusForbidden {
		t.Errorf("the REST door without send → %d, want 403", status)
	}

	sent := agent.CallOK(t, "send_outbound", map[string]any{"as": "direct_send", "id": review, "request_body": directed()}).Envelope(t).Data
	answerID(t, sent)

	var directedByAgent, adminEmail string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT i.directed_by_agent, u.email
		  FROM communication_instruction i JOIN app_user u ON u.id = i.directed_by
		 WHERE i.review_id = $1`, review).Scan(&directedByAgent, &adminEmail); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if adminEmail != "agent-direct-send@fable.test" {
		t.Errorf("the decision names %s, want the user who lent the passport", adminEmail)
	}
	if want := "agent:" + passport.String(); directedByAgent != want {
		t.Errorf("the decision names agent %q, want %q", directedByAgent, want)
	}

	var authority string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT execution_authority FROM comms_outbound`).Scan(&authority); err != nil {
		t.Fatalf("reading the delivery: %v", err)
	}
	if authority != "instruction" {
		t.Errorf("the delivery went out under %q, want instruction", authority)
	}
	var kept int
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT count(*) FROM communication_decision
		 WHERE phase = 'staging' AND verdict <> 'allow' AND execution_authority = 'instruction'`).Scan(&kept); err != nil {
		t.Fatalf("reading the decisions: %v", err)
	}
	if kept == 0 {
		t.Error("an agent's decision rewrote the refusal instead of standing over it")
	}
	var audited string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT actor_id FROM audit_log WHERE entity_type = 'communication_instruction' AND action = 'create'`).Scan(&audited); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if audited != directedByAgent {
		t.Errorf("the audit row names %q and the decision %q", audited, directedByAgent)
	}
	if _, err := d.e.Owner.Exec(t.Context(), `
		UPDATE communication_instruction SET directed_by_agent = NULL WHERE review_id = $1`, review); err == nil {
		t.Error("the agent's name on a decision was rewritten: it records who acted")
	}

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("the next correspondence → %d, want 409: the decision was spent", status)
	}
	own := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+own+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("the user's own decision → %d", status)
	}
	var byAgent *string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT directed_by_agent FROM communication_instruction WHERE review_id = $1`, own).Scan(&byAgent); err != nil {
		t.Fatalf("reading the user's decision: %v", err)
	}
	if byAgent != nil {
		t.Errorf("the user's own decision names agent %q", *byAgent)
	}
}

// The mail an agent would send is the mail the user would see in the composer.
func TestAnAgentPreviewsAContactsPrivacyMailAsTheUserDoes(t *testing.T) {
	d := newTwoDoorsWith(t, "agent-mail-preview", []compose.Option{compose.WithOperatorMail(apptest.DiscardingMailer{})}, "read")
	c := seedConsentSubject(t, d.e)
	var asUser map[string]any
	if status := d.e.Call(t, "GET", "/v1/contacts/"+c.contactID+"/consent/mail-preview?kind=privacy_notice", nil, nil, &asUser); status != http.StatusOK {
		t.Fatalf("the user's preview → %d", status)
	}
	var asAgent map[string]any
	raw := d.tool(t, "read_details", map[string]any{"action": "controller_mail_preview", "id": c.contactID, "kind": "privacy_notice"})
	if err := json.Unmarshal(raw, &asAgent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asUser, asAgent) {
		t.Errorf("the agent's preview %v differs from the user's %v", asAgent, asUser)
	}
}
