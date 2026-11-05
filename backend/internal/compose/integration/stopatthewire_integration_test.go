// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A stop committed after the consent gate answered and before the provider call
// reaches the message. The window is where pacing runs, so a policy records the
// subject's objection there, through the HTTP verb a rep uses.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/comms"
)

// objectionDuringPacing records the subject's objection the first time the
// dispatcher asks it how long to wait, and never makes the delivery wait.
type objectionDuringPacing struct {
	t        *testing.T
	p        *preflightEnv
	recorded bool
}

func (o *objectionDuringPacing) Name() string { return "objection recorded during pacing" }

func (o *objectionDuringPacing) Wait(context.Context, comms.Delivery) time.Duration {
	if !o.recorded {
		o.recorded = true
		if status := o.p.Call(o.t, "POST", "/v1/contacts/"+o.p.contactID+"/consent/suppress",
			AnyMap{"kind": "marketing_objection", "reason": "said stop while the message was queued"}, nil, nil); status != http.StatusNoContent {
			o.t.Errorf("recording the objection → %d, want 204", status)
		}
	}
	return 0
}

func TestAnObjectionCommittedBeforeTheWireStopsTheMessage(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	p.grantMarketingConsent(t)

	sentActivity := p.sendExpectingAcceptance(t, "marketing_email", "Spring pricing", "Here is what changed.")
	deliveryID, _ := p.deliveryFor(t, sentActivity)
	policy := &objectionDuringPacing{t: t, p: p}

	outcome, captured, _ := p.dispatchOnceWith(t, deliveryID, "", []comms.SendPolicy{policy})

	if !policy.recorded {
		t.Fatal("the policy never ran, so nothing was committed inside the window")
	}
	if outcome != comms.OutcomeParked {
		t.Fatalf("dispatch after an objection inside the window → %q, want parked", outcome)
	}
	if captured.raw != "" {
		t.Error("the message reached the provider although the subject objected before it was handed over")
	}
}
