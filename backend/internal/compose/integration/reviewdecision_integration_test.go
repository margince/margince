// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Asking somebody else to decide a refused send.
//
// Directing a send takes an authority most seats do not hold. Until now a rep
// without it could read their refusal and do nothing: the review recorded what
// was refused, and there was nowhere for "ask someone who can" to go.
//
// Routing is that ask. The review is staged as an approval card, and whoever
// approves it is the person whose name goes on the instruction — the asker's
// does not, because they did not decide anything.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// routedCard reads the card a routed review was staged as.
func routedCard(t *testing.T, c *consentEnv) (id, kind, state string) {
	t.Helper()
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT id::text, kind, status FROM approval ORDER BY created_at DESC LIMIT 1`).
		Scan(&id, &kind, &state); err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	return id, kind, state
}

// reviewState reads where the review stands.
func reviewState(t *testing.T, c *consentEnv) string {
	t.Helper()
	var state string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state FROM communication_review WHERE resolved_at IS NULL`).Scan(&state); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	return state
}

// A REFUSAL BECOMES A QUESTION SOMEBODY ELSE CAN ANSWER.
func TestARefusedSendCanBeHandedToSomebodyWhoMayDecideIt(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	var routed struct {
		ApprovalID string `json:"approval_id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{"note": "The service agreement obliges us to send this."}, nil, &routed); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}
	if routed.ApprovalID == "" {
		t.Fatal("routing returned no card, so the rep has asked nobody")
	}

	id, kind, _ := routedCard(t, c)
	if id != routed.ApprovalID {
		t.Errorf("the answer names card %s and the queue holds %s", routed.ApprovalID, id)
	}
	if kind != "communication_review" {
		t.Errorf("the card reads kind %q — a decider filtering their queue would not find it", kind)
	}
	// THE REVIEW SAYS IT IS SOMEBODY ELSE'S NOW. A rep who has asked should not
	// be shown a form they have already filled in.
	if state := reviewState(t, c); state != "awaiting_decision" {
		t.Errorf("the review reads %q after being routed, want awaiting_decision", state)
	}
}

// ASKING TWICE ASKS ONCE. A rep pressing the button again is asking one
// question about one refused message; two cards would put the same work in
// front of a decider twice.
func TestAskingTwiceRaisesOneCard(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	var first, second struct {
		ApprovalID string `json:"approval_id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, &first); status != http.StatusCreated {
		t.Fatalf("the first ask → %d, want 201", status)
	}
	status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{"note": "asking again"}, nil, &second)

	var cards int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'communication_review'`).Scan(&cards); err != nil {
		t.Fatalf("counting the cards: %v", err)
	}
	if cards != 1 {
		t.Errorf("%d card(s) after asking twice about one message, want 1 — a decider would be "+
			"shown the same refusal twice (second ask answered %d)", cards, status)
	}
}

// ASKING GRANTS NOTHING. The rep still cannot direct the send, and the message
// has not gone anywhere: routing makes the question findable and does not
// answer it.
func TestAskingForADecisionSendsNothing(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}

	var sent int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM comms_outbound`).Scan(&sent); err != nil {
		t.Fatalf("reading the deliveries: %v", err)
	}
	if sent != 0 {
		t.Errorf("%d message(s) went out on an ASK, want 0 — routing is a question, not a "+
			"decision", sent)
	}
	var instructions int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_instruction`).Scan(&instructions); err != nil {
		t.Fatalf("reading the decisions: %v", err)
	}
	if instructions != 0 {
		t.Errorf("%d decision(s) recorded by an ask, want 0 — the asker's name would be on an "+
			"override they never made", instructions)
	}
	// The message is still held, waiting for whoever answers.
	var held int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil {
		t.Fatalf("reading the held messages: %v", err)
	}
	if held != 1 {
		t.Errorf("%d held message(s) after routing, want 1 — the decision would have nothing to "+
			"act on", held)
	}
}

// A REVIEW THAT IS NOT WAITING FOR ONE CANNOT BE ROUTED. Directing a send
// already answered the question; raising a card afterwards would put a decided
// matter in front of somebody.
func TestADirectedReviewCannotThenBeRouted(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status == http.StatusCreated {
		t.Error("a review whose message has already gone was routed for a decision — somebody " +
			"would be asked to decide a matter that is closed")
	}
}

// ASKING AGAIN ANSWERS WITH THE CARD ALREADY CARRYING THE QUESTION.
//
// A rep whose first request timed out on the wire presses again. Refusing would
// tell them their ask failed when it did not, and leave them with no way to
// find the card they raised.
func TestAskingAgainAnswersWithTheStandingCard(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	var first, second struct {
		ApprovalID string `json:"approval_id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, &first); status != http.StatusCreated {
		t.Fatalf("the first ask → %d, want 201", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, &second); status != http.StatusCreated {
		t.Fatalf("asking again → %d, want 201 with the standing card", status)
	}
	if second.ApprovalID != first.ApprovalID {
		t.Errorf("the second ask answered card %q and the first raised %q — the rep cannot find "+
			"the card they already have", second.ApprovalID, first.ApprovalID)
	}
}

// A NOTE THE RECORD CANNOT HOLD IS REFUSED AT THE DOOR.
//
// The note becomes the instruction's explanation when somebody approves, and
// that column bounds it. A longer one would stage a card that cannot be
// approved: the decider presses approve, the redemption commits, and the send
// fails on a length nobody can see from the queue.
func TestANoteTooLongForTheRecordIsRefusedWhenItIsWritten(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/request-decision",
		AnyMap{"note": strings.Repeat("x", 1400)}, nil, nil); status == http.StatusCreated {
		t.Error("a note longer than the record accepts staged a card — approving it would fail " +
			"on a length nobody can see from the queue")
	}
	var cards int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'communication_review'`).Scan(&cards); err != nil {
		t.Fatalf("counting the cards: %v", err)
	}
	if cards != 0 {
		t.Errorf("%d card(s) staged for an unapprovable ask, want 0", cards)
	}
}

// DIRECTING FROM THE REVIEW RETRACTS THE CARD SOMEBODY WAS ASKED TO DECIDE.
//
// A rep can route their refusal and then be granted the authority, or a
// colleague can direct the same message from the review itself. The card is
// then asking about a message that has already gone, and approving it would put
// a second decision on the record for one send.
func TestDirectingAReviewRetractsTheCardItWasRoutedTo(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	var state string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT status FROM approval WHERE kind = 'communication_review'`).Scan(&state); err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	if state == "pending" {
		t.Error("the card is still waiting for a decision about a message that has already gone " +
			"— approving it would record a second decision for one send")
	}
}
