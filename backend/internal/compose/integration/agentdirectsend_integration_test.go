// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// The decision to send a refused message is the user's own. Their agent may
// read the review and put it to them, but has no action to decide it, and the
// route refuses its bearer. The user's own decision names no agent.
func TestOnlyTheUserDecidesARefusedSendNotTheirAgent(t *testing.T) {
	t.Parallel()
	d := newTwoDoorsWith(t, "agent-direct-send", []compose.Option{compose.WithOperatorMail(apptest.DiscardingMailer{})}, "read", "write")
	c := seedConsentSubject(t, d.e)
	c.useReviewableMarketingProfile(t)
	sendBearer, _ := apptest.PassportWithID(t, d.e, "direct-send agent", "read", "write", "send")
	agent := apptest.NewMCPClient(d.e, strings.TrimPrefix(sendBearer["Authorization"], "Bearer "))

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("correspondence with no supporting context → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	shown := string(agent.CallOK(t, "read_details", map[string]any{"action": "communication_review", "id": review}).Envelope(t).Data)
	if !strings.Contains(shown, consent.OverrideWarningVersion) {
		t.Fatalf("the review shows the agent no warning to put to its user: %s", shown)
	}

	if refused := agent.Call(t, "send_outbound", map[string]any{"as": "direct_send", "id": review, "request_body": directed()}); !refused.IsError {
		t.Errorf("an agent decided a refused send through send_outbound: %+v", refused)
	}
	if status := d.e.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), sendBearer, nil); status != http.StatusForbidden {
		t.Errorf("an agent's bearer on the route → %d, want 403", status)
	}
	var decisions int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM communication_instruction`).Scan(&decisions); err != nil {
		t.Fatalf("reading the decisions: %v", err)
	}
	if decisions != 0 {
		t.Fatalf("%d decision(s) recorded after only the agent asked, want 0", decisions)
	}

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("the user's own decision → %d, want 201", status)
	}
	var byAgent *string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT directed_by_agent FROM communication_instruction WHERE review_id = $1`, review).Scan(&byAgent); err != nil {
		t.Fatalf("reading the user's decision: %v", err)
	}
	if byAgent != nil {
		t.Errorf("the user's own decision names agent %q", *byAgent)
	}
}

// The mail an agent would send is the mail the user would see in the composer.
func TestAnAgentPreviewsAContactsPrivacyMailAsTheUserDoes(t *testing.T) {
	t.Parallel()
	// An hour off the wall clock, so a preview the clock never reached shows
	// another expiry. Its link must still be live.
	issued := clocktest.Now(t).Truncate(time.Minute).Add(-time.Hour)
	d := newTwoDoorsWith(t, "agent-mail-preview", []compose.Option{
		compose.WithConsentClockForTest(func() time.Time { return issued }),
		compose.WithOperatorMail(apptest.DiscardingMailer{}),
	}, "read")
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
	language, ok := asUser["language"].(string)
	if !ok {
		t.Fatalf("the preview names no language: %v", asUser)
	}
	// The seeded contact names no country and the installation keeps its UTC
	// default, so the link's fourteen days print in UTC.
	words := mailcopy.For(language)
	expiry := fmt.Sprintf(words.ConfirmExpiry, words.Instant(issued.Add(14*24*time.Hour)))
	if text, ok := asUser["text"].(string); !ok || !strings.Contains(text, expiry) {
		t.Errorf("the preview does not expire fourteen days after the pinned instant (%q): %q", expiry, text)
	}
}
