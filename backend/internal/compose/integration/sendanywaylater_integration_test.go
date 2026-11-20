// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// Send anyway with a moment records the decision now and sends nothing until
// the moment. The fire then spends the decision: one delivery, under it.
func TestSendAnywayWithAMomentWaitsAndSpendsTheDecisionAtTheFire(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)
	review := p.liveReviewFor(t, id)
	body := confirmationFor(t, p.AppEnv, review)
	body["scheduled_at"] = time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	body["scheduled_tz"] = "Europe/Berlin"
	var scheduled struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ScheduledAt string `json:"scheduled_at"`
	}
	if status := p.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", body, nil, &scheduled); status != http.StatusAccepted {
		t.Fatalf("Send anyway with a moment → %d, want 202", status)
	}
	if scheduled.ID != id.String() || scheduled.Status != activities.ScheduledStatusScheduled || scheduled.ScheduledAt == "" {
		t.Fatalf("the answer reads %+v, want message %s scheduled", scheduled, id)
	}
	if got := p.countDeliveries(t); got != 0 {
		t.Fatalf("%d deliveries before the moment, want 0", got)
	}
	if _, live := p.reviewsFor(t, id); live != 1 {
		t.Fatalf("%d live review(s) while the decided message waits, want 1", live)
	}

	p.makeDue(t, id)
	p.fire(t, id)
	var deliveries int
	var authority string
	if err := p.Owner.QueryRow(context.Background(),
		`SELECT count(*), coalesce(min(execution_authority), '') FROM comms_outbound`).Scan(&deliveries, &authority); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 || authority != "instruction" {
		t.Fatalf("%d deliveries under %q at the moment, want 1 under instruction", deliveries, authority)
	}
}

// A moment already past is refused before anything is recorded, and the
// message stays held.
func TestSendAnywayWithAPastMomentRecordsNothing(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)
	review := p.liveReviewFor(t, id)
	body := confirmationFor(t, p.AppEnv, review)
	body["scheduled_at"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	body["scheduled_tz"] = "UTC"
	if status := p.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("Send anyway with a past moment → %d, want 422", status)
	}
	var decisions int
	if err := p.Owner.QueryRow(context.Background(), `SELECT count(*) FROM communication_instruction`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 0 {
		t.Fatalf("%d decision(s) recorded for a refused moment, want 0", decisions)
	}
	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusHeld {
		t.Fatalf("the message reads %q, want still held", status)
	}
}

// A second Send anyway on a message that already went tells its sender so,
// instead of a not-found. A colleague still learns nothing.
func TestASecondSendAnywayHearsTheMessageAlreadyWent(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing with no grant → %d, want 409", status)
	}
	read := c.review(t, liveReviewID(t, c))
	path := "/v1/communication-reviews/" + read.ID + "/direct-send"
	if status := c.Call(t, "POST", path, confirmationOf(read), nil, nil); status != http.StatusCreated {
		t.Fatalf("the first tab → %d, want 201", status)
	}
	var again struct {
		Code string `json:"code"`
	}
	for _, body := range []AnyMap{confirmationOf(read), laterConfirmation(read)} {
		if status := c.Call(t, "POST", path, body, nil, &again); status != http.StatusConflict || again.Code != "already_sent" {
			t.Fatalf("the second tab → %d %q, want 409 already_sent", status, again.Code)
		}
	}
	if status := c.colleague(t).Call(t, "POST", path, confirmationOf(read), nil, nil); status != http.StatusNotFound {
		t.Fatalf("a colleague on the sent review → %d, want 404", status)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d deliveries, want 1", n)
	}
}

func laterConfirmation(read servedReview) AnyMap {
	body := confirmationOf(read)
	body["scheduled_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	body["scheduled_tz"] = "UTC"
	return body
}
