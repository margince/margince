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
	body["scheduled_at"] = momentIn(3 * time.Hour)
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
	body["scheduled_at"] = momentIn(-time.Hour)
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
	body["scheduled_at"] = momentIn(time.Hour)
	body["scheduled_tz"] = "UTC"
	return body
}

// Refusals that moved since the sender read the review answer a fresh review
// at once, and record no decision for a later moment.
func TestSendAnywayLaterOnMovedRefusalsAnswersAFreshReview(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing with no grant → %d, want 409", status)
	}
	read := c.review(t, liveReviewID(t, c))
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{
		"kind": "marketing_objection", "reason": "They objected on the phone after the review.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording the objection → %d", status)
	}
	var stale refusedSend
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", laterConfirmation(read), nil, &stale); status != http.StatusConflict {
		t.Fatalf("a later Send anyway on moved refusals → %d, want 409 with a fresh review", status)
	}
	if stale.Details.ReviewID == "" || stale.Details.ReviewID == read.ID {
		t.Fatalf("the 409 names review %q, want a fresh one beside %s", stale.Details.ReviewID, read.ID)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_instruction`); n != 0 {
		t.Fatalf("%d decision(s) on moved refusals, want 0", n)
	}
	if n := c.count(t, `SELECT count(*) FROM scheduled_send WHERE status = 'held'`); n != 1 {
		t.Fatalf("%d held message(s), want the one still held", n)
	}
}

// A message the engine now allows moves to its moment with no decision on
// record: nothing needs overriding.
func TestSendAnywayLaterRecordsNoDecisionTheEngineNoLongerNeeds(t *testing.T) {
	p, c, read := refusedOffer(t)
	p.grantMarketingConsent(t)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", laterConfirmation(read), nil, nil); status != http.StatusAccepted {
		t.Fatalf("a later Send anyway the engine now allows → %d, want 202", status)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_instruction`); n != 0 {
		t.Fatalf("%d decision(s) recorded for a message nothing refuses, want 0", n)
	}
	if n := c.count(t, `SELECT count(*) FROM scheduled_send WHERE status = 'scheduled'`); n != 1 {
		t.Fatalf("%d message(s) waiting for their moment, want 1", n)
	}
}

// A move that fails takes the decision with it: the two commit together.
func TestSendAnywayLaterThatCannotMoveRecordsNoDecision(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)
	review := p.liveReviewFor(t, id)
	body := confirmationFor(t, p.AppEnv, review)
	body["scheduled_at"] = momentIn(2 * time.Hour)
	body["scheduled_tz"] = "UTC"
	for _, ddl := range []string{
		`CREATE FUNCTION refuse_later_move() RETURNS trigger LANGUAGE plpgsql
		 AS $$ BEGIN RAISE EXCEPTION 'the move is interrupted by the test'; END $$`,
		`CREATE TRIGGER refuse_later_move BEFORE UPDATE ON scheduled_send
		 FOR EACH ROW WHEN (NEW.status = 'scheduled') EXECUTE FUNCTION refuse_later_move()`,
	} {
		if _, err := p.Owner.Exec(t.Context(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := p.Owner.Exec(context.Background(), `DROP FUNCTION IF EXISTS refuse_later_move() CASCADE`); err != nil {
			t.Error(err)
		}
	})
	if status := p.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", body, nil, nil); status < 500 {
		t.Fatalf("the interrupted move → %d, want a server fault", status)
	}
	if n := countRows(t, p, `SELECT count(*) FROM communication_instruction`); n != 0 {
		t.Fatalf("%d decision(s) left by a move that failed, want 0", n)
	}
	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusHeld {
		t.Fatalf("the message reads %q, want still held", status)
	}
}

// A message its sender cancelled was never sent: Send anyway on its review is
// not found, not "already sent".
func TestSendAnywayOnACancelledMessageIsNotFound(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing with no grant → %d, want 409", status)
	}
	read := c.review(t, liveReviewID(t, c))
	held := heldSends(t, c.AppEnv)
	if len(held) != 1 {
		t.Fatalf("%d held messages, want 1", len(held))
	}
	if status := c.Call(t, "POST", "/v1/scheduled-sends/"+held[0].id+"/cancel", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("cancelling → %d, want 204", status)
	}
	var answer struct {
		Code string `json:"code"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", confirmationOf(read), nil, &answer); status != http.StatusNotFound {
		t.Fatalf("Send anyway on a cancelled message → %d %q, want 404", status, answer.Code)
	}
}

func countRows(t *testing.T, p *preflightEnv, query string) int {
	t.Helper()
	var n int
	if err := p.Owner.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// momentIn is a send time relative to the server's own clock, which is what
// the server checks a moment against.
func momentIn(d time.Duration) string {
	return time.Now().Add(d).UTC().Format(time.RFC3339)
}
