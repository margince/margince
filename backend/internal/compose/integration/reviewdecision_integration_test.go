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
// approves it is the human whose name goes on the instruction — the asker's
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

// A DECIDER CAN READ THE REFUSAL THEY ARE BEING ASKED ABOUT.
//
// This is the gap the routing slice left. The approve button worked and the
// thing it was about answered 404, because reading a review was scoped to the
// person who pressed send. A reviewer acknowledged a warning about a message
// they had never seen, which is the one thing an acknowledgement must not be.
func TestADeciderCanReadTheRefusalTheyAreAskedAbout(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}

	// The review is readable, and it carries what a decider needs: what was
	// refused, for whom, and the card the question is on.
	var opened struct {
		State      string `json:"state"`
		ReasonCode string `json:"reason_code"`
		ApprovalID string `json:"approval_id"`
		Refusals   []struct {
			Address string `json:"address"`
		} `json:"refusals"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &opened); status != http.StatusOK {
		t.Fatalf("reading the routed review → %d, want 200 — a decider cannot see what they are "+
			"being asked to decide", status)
	}
	if opened.State != "awaiting_decision" {
		t.Errorf("the review reads %q, want awaiting_decision", opened.State)
	}
	if len(opened.Refusals) == 0 {
		t.Error("the review names no recipient, so a decider is told a message was refused and " +
			"not who for")
	}
	if opened.ApprovalID == "" {
		t.Error("the review names no card, so a decider reading it cannot find the decision they " +
			"are being asked to make")
	}
}

// THE QUEUE SHOWS WHAT IS WAITING.
func TestTheDecidersQueueListsWhatIsWaiting(t *testing.T) {
	c := setupConsent(t)

	var listed struct {
		Data  []struct{ ID string } `json:"data"`
		Total int                   `json:"total"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("reading the queue → %d, want 200", status)
	}
	if listed.Total != 0 {
		t.Fatalf("%d waiting before anything was routed, want 0", listed.Total)
	}

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	// A refusal nobody has asked about is NOT in the decider's queue. It is the
	// rep's own work until they hand it on.
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("reading the queue → %d", status)
	}
	if listed.Total != 0 {
		t.Errorf("%d waiting before the rep asked anybody, want 0 — an unrouted refusal is not "+
			"somebody else's work", listed.Total)
	}

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("reading the queue → %d", status)
	}
	if listed.Total != 1 || len(listed.Data) != 1 {
		t.Fatalf("%d waiting after one ask (%d listed), want 1", listed.Total, len(listed.Data))
	}
	if listed.Data[0].ID != review {
		t.Errorf("the queue holds review %s and %s was routed", listed.Data[0].ID, review)
	}

	// Once the message goes, the work is done and the queue says so.
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("reading the queue → %d", status)
	}
	if listed.Total != 0 {
		t.Errorf("%d still waiting after the message went, want 0 — a decider would be shown "+
			"work that is done", listed.Total)
	}
}

// A SEAT THAT CANNOT DECIDE SEES NOTHING, and that is the disclosure question
// this queue turns on: the rows name other people's refused correspondence.
func TestASeatThatCannotDecideIsNotShownTheQueue(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("routing the review → %d, want 201", status)
	}

	// The seat loses the grant that makes somebody a decider.
	if _, err := c.Owner.Exec(context.Background(), `
		UPDATE role SET permissions = jsonb_set(
			permissions, '{objects,communication_exception}',
			'{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)`); err != nil {
		t.Fatalf("removing the grant: %v", err)
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, nil); status == http.StatusOK {
		t.Error("a seat that cannot direct a send was handed the queue of refused messages — " +
			"other people's correspondence, to somebody with no reason to see it")
	}
}

// READING AND ROUTING ARE DIFFERENT DOORS, and routing is the narrow one.
//
// A decider must be able to READ any refusal they are asked about — that is the
// gap this slice closed. Routing is not the same act: a seat that could route
// anybody's review would be raising cards about other people's correspondence,
// and somebody holding the exception grant can direct the send themselves
// rather than asking a colleague to.
//
// So routing stays bound to the person whose message it was. This pins that: a
// review belonging to nobody in this session cannot be routed, even by a seat
// that may read every review in the installation.
func TestRoutingStaysBoundToThePersonWhoseMessageItWas(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	// The review is re-pointed at somebody else. The caller keeps the exception
	// grant — so they can still READ it — and is no longer its initiator.
	var other string
	if err := c.Owner.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name, status, seat_type, password_hash)
		VALUES ('other-rep@consent.test', 'Other Rep', 'active', 'full', 'x')
		RETURNING id::text`).Scan(&other); err != nil {
		t.Fatalf("creating the other seat: %v", err)
	}
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_review SET initiated_by = $1 WHERE id = $2`, other, review); err != nil {
		t.Fatalf("re-pointing the review: %v", err)
	}

	// Readable, because the caller may decide refused sends.
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("reading somebody else's review as a decider → %d, want 200", status)
	}
	// And not routable, because it is not their message to hand on.
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status == http.StatusCreated {
		t.Error("a seat routed somebody else's refusal — raising a card about correspondence " +
			"that is not theirs")
	}
}
