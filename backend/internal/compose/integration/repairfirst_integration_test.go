// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// refusedWithActions is a refused send's 409 with the actions it offers.
type refusedWithActions struct {
	Code    string `json:"code"`
	Details struct {
		ReviewID         string   `json:"review_id"`
		AvailableActions []string `json:"available_actions"`
	} `json:"details"`
}

// offerTo sends a reviewable marketing message to one address.
func (p *preflightEnv) offerTo(t *testing.T, address string) (int, refusedWithActions) {
	t.Helper()
	var refused refusedWithActions
	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Our offer", "body": "The terms for next year.",
		"to": []string{address}, "consent_purpose": "marketing_email",
	}, nil, &refused)
	return status, refused
}

// bounceHard delivers one reply to the buyer and reports it hard-bounced
// through the comms store the bounce sink composes.
func (p *preflightEnv) bounceHard(t *testing.T) {
	t.Helper()
	var reply struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+logConsentInbound(t, p.AppEnv, p.contactID)+"/send-email", AnyMap{
		"subject": "Re: Termin", "body": exitReschedule,
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "business_correspondence",
	}, nil, &reply); status != http.StatusAccepted {
		t.Fatalf("the reply → %d, want 202", status)
	}
	activity, err := ids.Parse(reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, messageID := p.deliveryFor(t, activity)
	p.transmit(t, delivery, "")
	user, err := ids.Parse(p.user)
	if err != nil {
		t.Fatal(err)
	}
	reporter := principal.WithCorrelationID(principal.WithActor(
		principal.WithWorkspaceID(context.Background(), p.workspaceID(t)),
		principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: user, OnBehalfOf: user},
	), ids.NewV7())
	db := compose.InstallationDB(p.Pool)
	store := comms.NewStore(db, func() time.Time { return clocktest.Now(t) }, activities.NewStore(db), comms.WithBounceObserver(bounceToConsent{}))
	if _, err := store.RecordBounce(reporter, connector.BounceReport{
		MessageID: messageID, Recipient: "buyer@preflight.test", Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil {
		t.Fatalf("reporting the bounce: %v", err)
	}
}

// A message with a technical fault and a policy concern asks for the repair
// first: the mailbox, then the dead address. Only then does the policy concern
// offer Send anyway.
func TestTheRepairComesBeforeThePolicyDecision(t *testing.T) {
	t.Parallel()
	p := setupPreflight(t)
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	c.useReviewableMarketingProfile(t)

	var broken struct {
		Code string `json:"code"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Our offer", "body": "The terms for next year.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "marketing_email",
	}, nil, &broken); status != http.StatusUnprocessableEntity || broken.Code != "mailbox_not_connected" {
		t.Fatalf("an offer through no mailbox → %d %q, want the mailbox repair", status, broken.Code)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_review`); n != 0 {
		t.Fatalf("%d reviews opened behind a broken mailbox, want none", n)
	}

	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.bounceHard(t)
	status, dead := p.offerTo(t, "buyer@preflight.test")
	if status != http.StatusConflict || slices.Contains(dead.Details.AvailableActions, "direct_send") {
		t.Fatalf("an offer to a dead address → %d offering %v, want a 409 without Send anyway", status, dead.Details.AvailableActions)
	}
	if lead := c.review(t, dead.Details.ReviewID).reasons(); len(lead) == 0 || lead[0] != commsauthz.ReasonHardBounce {
		t.Fatalf("the review leads with %v, want the bounce", lead)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+dead.Details.ReviewID+"/direct-send",
		confirmationOf(c.review(t, dead.Details.ReviewID)), nil, nil); status != http.StatusForbidden {
		t.Fatalf("Send anyway past a dead address → %d, want 403", status)
	}

	const corrected = "buyer.corrected@preflight.test"
	if status := c.Call(t, "PATCH", "/v1/contacts/"+c.contactID, AnyMap{
		"emails": []AnyMap{{"email": corrected, "is_primary": true}},
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("correcting the address → %d", status)
	}
	status, policy := p.offerTo(t, corrected)
	if status != http.StatusConflict || !slices.Contains(policy.Details.AvailableActions, "direct_send") {
		t.Fatalf("the offer to the corrected address → %d offering %v, want a 409 with Send anyway", status, policy.Details.AvailableActions)
	}
	read := c.review(t, policy.Details.ReviewID)
	if slices.Contains(read.reasons(), commsauthz.ReasonHardBounce) {
		t.Fatalf("the repaired message still names the bounce: %v", read.reasons())
	}
	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", confirmationOf(read), nil, &sent); status != http.StatusCreated {
		t.Fatalf("Send anyway on the policy concern → %d, want 201", status)
	}
	activity, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	delivery, _ := p.deliveryFor(t, activity)
	if mime, _ := p.transmit(t, delivery, ""); !strings.Contains(string(mime), corrected) {
		t.Fatalf("the delivered copy is not addressed to %s:\n%s", corrected, mime)
	}
}
