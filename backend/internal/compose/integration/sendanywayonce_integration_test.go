// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refusedOffer is a reviewable marketing message the engine refused, read as
// its sender is served it.
func refusedOffer(t *testing.T) (*preflightEnv, *consentEnv, servedReview) {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
	c.useReviewableMarketingProfile(t)
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Our offer", "body": "The terms you asked about.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "marketing_email",
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("marketing with no grant → %d, want 409", status)
	}
	return p, c, c.review(t, liveReviewID(t, c))
}

// Send anyway pressed in two tabs at once is one decision and one queued
// message, and a worker that runs twice transmits it once.
func TestSendAnywayFromTwoTabsQueuesAndDeliversOneMessage(t *testing.T) {
	p, c, read := refusedOffer(t)
	path := "/v1/communication-reviews/" + read.ID + "/direct-send"

	statuses := make([]int, 2)
	var tabs sync.WaitGroup
	for tab := range statuses {
		tabs.Go(func() {
			statuses[tab] = c.Call(t, "POST", path, confirmationOf(read), nil, nil)
		})
	}
	tabs.Wait()
	slices.Sort(statuses)
	if statuses[0] != http.StatusCreated || statuses[1] < 400 || statuses[1] >= 500 {
		t.Fatalf("two tabs answered %v, want one 201 and one refusal", statuses)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d messages queued by two tabs, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_instruction`); n != 1 {
		t.Fatalf("%d decisions recorded by two tabs, want 1", n)
	}

	var delivery ids.UUID
	if err := p.Owner.QueryRow(t.Context(), `SELECT id FROM comms_outbound`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	p.transmit(t, delivery, "")
	if outcome, again, _ := p.dispatchOnce(t, delivery, ""); outcome == comms.OutcomeSent || again.raw != "" {
		t.Fatalf("a second worker run → %q, reached the provider: %t", outcome, again.raw != "")
	}
}

// A send or a Send anyway retried under its Idempotency-Key, as a client does
// after a timeout, answers the recorded activity and queues nothing more.
func TestASendRetriedUnderItsKeyQueuesOneMessage(t *testing.T) {
	p, c, read := refusedOffer(t)
	p.stakeADeal(t)
	key := map[string]string{"Idempotency-Key": "send-" + read.ID}
	reply := AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
	}
	var first, retried struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", reply, key, &first); status != http.StatusAccepted {
		t.Fatalf("the send → %d, want 202", status)
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", reply, key, &retried); status != http.StatusAccepted || retried.ID != first.ID {
		t.Fatalf("the retry → %d %q, want the recorded 202 %q", status, retried.ID, first.ID)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d messages queued after the retried send, want 1", n)
	}

	path := "/v1/communication-reviews/" + read.ID + "/direct-send"
	anyway := map[string]string{"Idempotency-Key": "send-anyway-" + read.ID}
	var decided, redecided struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", path, confirmationOf(read), anyway, &decided); status != http.StatusCreated {
		t.Fatalf("Send anyway → %d, want 201", status)
	}
	if status := c.Call(t, "POST", path, confirmationOf(read), anyway, &redecided); status != http.StatusCreated || redecided.ID != decided.ID {
		t.Fatalf("the retried Send anyway → %d %q, want the recorded 201 %q", status, redecided.ID, decided.ID)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 2 {
		t.Fatalf("%d messages queued after the retried Send anyway, want 2", n)
	}
}
