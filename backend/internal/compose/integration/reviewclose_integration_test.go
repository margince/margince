// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What closes a review, asked from every direction.
//
// A refusal freezes a message into a held row and opens a review for it. The
// review is over once the message's fate is settled — and until the slice these
// tests ship with, only ONE of the ways that happens actually closed it.
//
// The three ways a held message settles:
//
//   - it is CANCELLED, so nobody will ever send it;
//   - its recipient is ERASED, so there is no message and nobody to send it to;
//   - it is RESUMED and the engine now ALLOWS it, so it goes out on the
//     engine's own permission and consumes no instruction.
//
// Every one of them left a live review behind, showing a decider work about a
// message that is cancelled, gone or already delivered — and the approval CARD
// stayed pending too, so somebody could still press approve and mint an
// instruction for a send that cannot happen or has already happened.
//
// RESCHEDULING looks like a fourth and is not. A moved message is still going
// out and its decision is still outstanding, and the path that refires it opens
// no review of its own — so closing on a move would lose the refusal entirely.
// The test for it pins the opposite of the other three.
//
// Each test below drives one direction through the real handlers and asserts
// both halves.

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// heldMessageID answers the message this installation's one refusal froze.
func heldMessageID(t *testing.T, c *consentEnv) string {
	t.Helper()
	var id string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM scheduled_send WHERE status = 'held'`).Scan(&id); err != nil {
		t.Fatalf("finding the held message: %v", err)
	}
	return id
}

// closedReview reads where a review ended and whether it says when.
func closedReview(t *testing.T, c *consentEnv) (state string, resolved *string) {
	t.Helper()
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state, resolved_at::text FROM communication_review`).Scan(&state, &resolved); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	return state, resolved
}

// cardStatus reads the approval a routed review was staged as.
func cardStatus(t *testing.T, c *consentEnv) string {
	t.Helper()
	var status string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT status FROM approval WHERE kind = 'communication_review'`).Scan(&status); err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	return status
}

// refuseAndRoute is the shared opening every test below needs: a marketing send
// the engine refuses, and the review handed to somebody who may decide it.
//
// ROUTED IN EVERY CASE, because the card is the half that was missing. A test
// that only refused would assert the review closes and never notice that a
// decider is still holding a pending question about it.
func refuseAndRoute(t *testing.T, c *consentEnv) (review, held string) {
	t.Helper()
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	review = liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}
	if got := cardStatus(t, c); got != "pending" {
		t.Fatalf("the routed card reads %q, want pending — this test has nothing to prove "+
			"unless a decider really is being asked", got)
	}
	return review, heldMessageID(t, c)
}

// CANCELLING A MESSAGE RETRACTS THE CARD ASKING WHETHER IT MAY GO.
//
// The cancel already closed the review before this slice. What it did not do
// was reach the card: CancelReviewForIntentTx was a free function with no
// router in hand, so a routed review went to 'cancelled' while the approval
// stayed pending. A decider pressing approve then ran the directed send effect
// against a message that had been cancelled — and it failed at redemption
// rather than being pre-empted, which is a decider told the system is broken
// when what really happened is that somebody withdrew the message.
func TestCancellingAMessageRetractsTheCardAskingAboutIt(t *testing.T) {
	c := setupConsent(t)
	_, held := refuseAndRoute(t, c)

	if status := c.Call(t, "POST", "/v1/scheduled-sends/"+held+"/cancel",
		AnyMap{}, nil, nil); status >= 400 {
		t.Fatalf("cancelling the held message → %d", status)
	}

	state, resolved := closedReview(t, c)
	if state != "cancelled" {
		t.Errorf("the review reads %q after its message was cancelled, want cancelled", state)
	}
	if resolved == nil {
		t.Error("the review says it is finished and names no moment, which its shape check refuses")
	}
	if got := cardStatus(t, c); got == "pending" {
		t.Error("the card is still asking whether a CANCELLED message may go — approving it " +
			"would direct a send that cannot happen, and the decider would be shown a failure " +
			"rather than an answer")
	}
}

// MOVING A MESSAGE LEAVES ITS REVIEW LIVE, BECAUSE THE DECISION IS STILL OPEN.
//
// This test pins a decision that was reversed while the slice was being
// reviewed. Closing the review on a reschedule reads right — nothing is sent
// now, and the row is 'scheduled' again — and it loses the refusal.
//
// The reason is that only the STAGING path opens a review, through
// RecordPendingReview. The timer-driven refire holds a refused message
// (scheduledsendfire.go, holdInTx) and opens none. So a review closed here and
// the message refused again at its new moment would leave it held with nothing
// routable in front of anybody, which is the silence the review exists to end.
//
// Left live, the row stays honest: the message really is still going out and
// the decision really is still outstanding. If it is refused again,
// OpenReviewTx upserts on the live-intent index and refreshes THIS row rather
// than opening a rival.
func TestMovingAMessageLeavesItsReviewLive(t *testing.T) {
	c := setupConsent(t)
	_, held := refuseAndRoute(t, c)

	// The move is version-checked, like every write on this row: the caller
	// states which version they were looking at.
	var current struct {
		Version int64 `json:"version"`
	}
	if status := c.Call(t, "GET", "/v1/scheduled-sends/"+held, nil, nil, &current); status != http.StatusOK {
		t.Fatalf("reading the held message → %d", status)
	}
	when := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	var problem map[string]any
	if status := c.Call(t, "PATCH", "/v1/scheduled-sends/"+held,
		AnyMap{"scheduled_at": when, "scheduled_tz": "UTC"},
		map[string]string{"If-Match": strconv.FormatInt(current.Version, 10)},
		&problem); status >= 400 {
		t.Fatalf("moving the held message to %s → %d: %v", when, status, problem)
	}

	// The message really did move, so this test is about a live message rather
	// than one that quietly failed to reschedule.
	var status string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT status FROM scheduled_send WHERE id = $1`, held).Scan(&status); err != nil {
		t.Fatalf("reading the moved message: %v", err)
	}
	if status != "scheduled" {
		t.Fatalf("the message reads %q after being moved, want scheduled", status)
	}

	_, resolved := closedReview(t, c)
	if resolved != nil {
		t.Error("the review closed when its message was merely moved — the message is still " +
			"going out, and the refire that carries it opens no review of its own, so the " +
			"refusal is now invisible to everybody")
	}
	if got := cardStatus(t, c); got != "pending" {
		t.Errorf("the card reads %q after the message was moved, want pending — the decider is "+
			"still being asked a question that is still open", got)
	}
}

// ERASING THE RECIPIENT ENDS THE REVIEW AND THE CARD.
//
// The erasure already cancelled the held row and emptied the review's refusals.
// What it left was the review's LIVENESS and its card: an erased subject's
// refused message still readable as work, with a pending approval asking
// whether to send it. Approving that fires an emptied message at an emptied
// address list, from a system that has just certified the subject's data
// destroyed.
//
// The card is reached HERE and nowhere else. The erasure's own approval sweep
// finds cards by the subject they name — contact target, lead twin, or an
// address quoted in the payload — and a review card names none of those: its
// payload is the review id and the intent id, because the question is "may this
// refused send go" and never repeats who it was to.
func TestErasingTheRecipientEndsTheReviewAndItsCard(t *testing.T) {
	c := setupConsent(t)
	refuseAndRoute(t, c)

	// Through the real Art. 17 case, over the same handlers a DPO uses:
	// fulfilling an erasure request is what runs the destructive engine.
	var request struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind":        "erasure",
		"subject_ref": c.contactID,
		"due_at":      time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, nil, &request); status != http.StatusCreated {
		t.Fatalf("opening the erasure case → %d, want 201", status)
	}
	var dsrProblem map[string]any
	if status := c.Call(t, "PATCH", "/v1/data-subject-requests/"+request.ID,
		AnyMap{"status": "fulfilled", "resolution": "erased on the subject's request"},
		nil, &dsrProblem); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure case → %d, want 200: %v", status, dsrProblem)
	}

	// The engine really ran, so this test is about what an erasure leaves
	// behind rather than about a case that was merely marked done.
	var payload string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT payload->>'subject' FROM scheduled_send`).Scan(&payload); err != nil {
		t.Fatalf("reading the erased message: %v", err)
	}
	if payload != "" {
		t.Fatalf("the held message still reads %q, so the erasure did not reach it and this "+
			"test proves nothing about what it leaves behind", payload)
	}

	state, resolved := closedReview(t, c)
	if resolved == nil {
		t.Errorf("the review is still live (%q) over an erased subject's cancelled message — "+
			"somebody can still direct a send to an address that no longer exists", state)
	}
	if state == "resolved" {
		t.Error("an erased subject's message left a RESOLVED review, which reads as a message " +
			"that went out")
	}
	if got := cardStatus(t, c); got == "pending" {
		t.Error("the card is still asking whether to send a message to an erased subject")
	}
}

// A MESSAGE THE ENGINE COMES TO ALLOW CLOSES ITS OWN REVIEW.
//
// THE HOLE THAT READS LIKE SUCCESS. Consuming an instruction resolves the
// review it answered — but staging only asks for an instruction when it
// actually refuses somebody (authorizestaging.go: `refusesAnyRecipient(set)`).
// A resume whose staging now ALLOWS every recipient — the grant arrived, the
// stop was lifted, the cap rolled over — consumes nothing, so it took that
// path's exit without passing its door.
//
// The message went out and its review stayed live, showing a decider a refusal
// about a message already in somebody's inbox. Approving that card mints an
// instruction for a delivery that has already been made.
func TestAMessageTheEngineComesToAllowClosesItsOwnReview(t *testing.T) {
	c := setupConsent(t)
	review, _ := refuseAndRoute(t, c)

	// The reason for the refusal goes away: the subject grants the marketing
	// purpose through the real public confirm link.
	c.grantMarketingByConfirmLink(t)

	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, &sent); status != http.StatusCreated {
		t.Fatalf("sending the now-allowed message → %d, want 201", status)
	}
	if sent.ID == "" {
		t.Fatal("nothing went out, so this test is not about a message that was sent")
	}

	// NO INSTRUCTION WAS CONSUMED, which is the whole premise: staging allowed
	// every recipient, so it never asked for one. If this stops being true the
	// test is passing for the directed path's reason rather than this one.
	var consumed int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_instruction WHERE consumed_at IS NOT NULL`).
		Scan(&consumed); err != nil {
		t.Fatalf("counting spent decisions: %v", err)
	}
	if consumed != 0 {
		t.Fatalf("%d instruction(s) were spent on a message the engine allowed: this test no "+
			"longer exercises the path it was written for", consumed)
	}

	state, resolved := closedReview(t, c)
	if resolved == nil {
		t.Errorf("the review is still live (%q) about a message that has already been sent — "+
			"a decider approving it mints an override for a delivery that has been made", state)
	}
	// RESOLVED, not cancelled: this one did go out, and that distinction is
	// what lets a reader tell a sent message from an abandoned one.
	if resolved != nil && state != "resolved" {
		t.Errorf("the sent message left a %q review, want resolved", state)
	}
	if got := cardStatus(t, c); got == "pending" {
		t.Error("the card is still asking whether a message that has already been sent may go")
	}
}
