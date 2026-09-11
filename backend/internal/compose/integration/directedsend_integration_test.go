// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A designated human sending a message the engine refused.
//
// This is the end of the chain the last three slices built. The engine refuses
// and records why. The refusal freezes its message into a held row and opens a
// review bound to it. A named human reads that review, acknowledges a warning,
// says in writing why this message goes — and the exact message goes.
//
// WHAT MUST REMAIN TRUE AFTERWARDS is the whole point, and it is what these
// tests hold: the refusal is still a refusal, the suppression is untouched, the
// decision rows still read `deny`, and the next message to the same person is
// refused all over again. An override that quietly became a consent grant would
// be worse than no override at all.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// directed is the request body a designated human sends.
func directed() AnyMap {
	return AnyMap{
		"reason_code":     "contractual_necessity",
		"explanation":     "The service agreement obliges us to send this notice.",
		"warning_version": "override-v1",
		"acknowledged":    true,
	}
}

// liveReviewID answers the review this installation's one refusal opened.
func liveReviewID(t *testing.T, c *consentEnv) string {
	t.Helper()
	var id string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM communication_review WHERE resolved_at IS NULL`).Scan(&id); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	return id
}

// THE MESSAGE GOES, AND THE REFUSAL STAYS A REFUSAL.
func TestADesignatedUserSendsTheMessageTheEngineRefused(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, &sent); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}
	if sent.ID == "" {
		t.Fatal("the directed send produced no activity, so nothing went out")
	}

	// ONE delivery, and it names the decision it went out under.
	var deliveries int
	var authority string
	var instruction *string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*), min(execution_authority), min(instruction_id::text)
		  FROM comms_outbound`).Scan(&deliveries, &authority, &instruction); err != nil {
		t.Fatalf("reading the deliveries: %v", err)
	}
	if deliveries != 1 {
		t.Errorf("%d deliveries for one directed message, want 1", deliveries)
	}
	if authority != "instruction" {
		t.Errorf("the delivery went out under %q, want instruction — a message sent on a "+
			"person's decision that records the engine's permission attributes it to nobody",
			authority)
	}
	if instruction == nil || *instruction == "" {
		t.Error("the delivery names no decision, so nothing says who authorized it")
	}

	// THE ENGINE'S ANSWER IS UNCHANGED. This is the invariant the whole design
	// rests on: rewriting the verdict to allow would erase the only record that
	// anybody overrode anything.
	var denied int
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_decision
		 WHERE phase = 'staging' AND verdict <> 'allow' AND execution_authority = 'instruction'`).
		Scan(&denied); err != nil {
		t.Fatalf("reading the decisions: %v", err)
	}
	if denied == 0 {
		t.Error("no refused decision row names the instruction — the refusal was rewritten " +
			"rather than overridden, so nothing records that anybody overrode it")
	}
}

// THE DECISION IS SPENT, AND THE NEXT MESSAGE NEEDS ITS OWN.
//
// An instruction authorizes one message to one envelope. If it survived its own
// use, a single acknowledgement would license every future message to that
// person — which is a standing consent grant nobody gave, wearing an override's
// name.
func TestTheNextMessageToTheSamePersonNeedsItsOwnDecision(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("first marketing send → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	var status string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT status FROM communication_instruction`).Scan(&status); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if status != "consumed" {
		t.Errorf("the decision reads %q after being acted on, want consumed — a decision that "+
			"survives its own use licenses every later message to the same person", status)
	}

	// A fresh message to the same person: refused again, exactly as before.
	if code, _ := c.send(t, "marketing_email"); code != http.StatusConflict {
		t.Errorf("the next marketing send → %d, want 409 — the override leaked into a standing "+
			"permission", code)
	}
}

// THE SUPPRESSION IS NEVER TOUCHED. A subject who asked us to stop has asked us
// to stop; an override sends one message against that and changes nothing about
// the standing request.
func TestDirectingASendLeavesTheSubjectsStopExactlyWhereItWas(t *testing.T) {
	c := setupConsent(t)

	if _, err := c.Owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (person_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'operator_ui', 'test', 'subject')`,
		c.personID); err != nil {
		t.Fatalf("recording the subject's stop: %v", err)
	}
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send against a stop → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	var live int
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE person_id = $1 AND kind = 'subject_request' AND revoked_at IS NULL`,
		c.personID).Scan(&live); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if live != 1 {
		t.Errorf("%d live stop(s) after a directed send, want 1 — sending against a stop lifted "+
			"it, so the subject's request was answered by ignoring it", live)
	}
	var grants int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM person_consent WHERE person_id = $1`, c.personID).Scan(&grants); err != nil {
		t.Fatalf("reading the consent rows: %v", err)
	}
	if grants != 0 {
		t.Errorf("%d consent grant(s) after a directed send, want 0 — an override was recorded "+
			"as permission the subject never gave", grants)
	}
}

// AN UNACKNOWLEDGED DECISION IS NOT A DECISION. The tick is the act.
func TestAnUnacknowledgedDirectionSendsNothing(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	body := directed()
	body["acknowledged"] = false
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/direct-send",
		body, nil, nil); status == http.StatusCreated {
		t.Fatal("an unacknowledged direction sent the message — the acknowledgement is the act, " +
			"and a record written without one says a person decided something they did not")
	}
	var sent int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM comms_outbound`).Scan(&sent); err != nil {
		t.Fatalf("reading the deliveries: %v", err)
	}
	if sent != 0 {
		t.Errorf("%d message(s) went out on an unacknowledged direction, want 0", sent)
	}
}

// A DECISION IS SPENT ON ONE MESSAGE AND CANNOT BE RE-POINTED AT ANOTHER.
//
// The record says a named person decided that THIS message goes. If the row
// could later be made to name a different delivery, the account of who
// authorized what would be rewritable by whoever wanted it rewritten — which is
// the one thing a dispute about an override needs not to be possible.
//
// Held by the database rather than by the code that writes it, because the code
// is what a mistake would be in.
func TestASpentDecisionCannotBeRePointedAtAnotherMessage(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	_, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_instruction SET delivery_id = gen_random_uuid()`)
	if err == nil {
		t.Fatal("a spent decision was re-pointed at another message — the record of who " +
			"authorized what is rewritable")
	}
	// And the acknowledgement itself is frozen, for the same reason.
	_, err = c.Owner.Exec(context.Background(),
		`UPDATE communication_instruction SET acknowledged_wording = sha256('anything')`)
	if err == nil {
		t.Error("the fingerprint of the acknowledged message was rewritten — a decision could " +
			"then be made to describe a message nobody read")
	}
}

// A SENT MESSAGE CLOSES THE REVIEW IT ANSWERED.
//
// Without this the review stays live forever: the rep's queue keeps showing
// work that is done, the message can still be routed to a decider who would be
// asked about a closed matter, and the index that allows one live review per
// held message goes on treating a spent one as waiting.
//
// Resolved, not deleted — a subject asking why they received the message is
// shown the review beside the decision, and a queue that emptied itself by
// forgetting would answer nothing.
func TestADirectedSendClosesTheReviewItAnswered(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("directing the send → %d, want 201", status)
	}

	var state string
	var resolved *string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state, resolved_at::text FROM communication_review WHERE id = $1`, review).
		Scan(&state, &resolved); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	if state != "resolved" {
		t.Errorf("the review reads %q after its message went, want resolved — the rep's queue "+
			"still shows work that is done", state)
	}
	if resolved == nil {
		t.Error("the review says it is finished and names no moment, which the row's own shape " +
			"check refuses")
	}
	// It is still readable. The record of what was refused is what a subject
	// asking about the message is shown.
	var rows int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_review WHERE id = $1`, review).Scan(&rows); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	if rows != 1 {
		t.Error("the review was deleted rather than closed — nothing is left to answer a subject " +
			"asking why they received the message")
	}
}

// THE WARNING IS THE SERVER'S OWN WORDS.
//
// An instruction's whole claim is that a named person read a particular
// caution before overruling the engine, and it records that as a version. A
// client free to compose its own wording, or to name any version it liked,
// would have the record assert an acknowledgement of text nobody published.
//
// So the review serves the words and the version together, and the door refuses
// a version this installation does not serve.
func TestTheWarningADirectorAcknowledgesIsTheServersOwn(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	var opened struct {
		Warning struct {
			Version string `json:"version"`
			Text    string `json:"text"`
		} `json:"warning"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &opened); status != http.StatusOK {
		t.Fatalf("reading the review → %d, want 200", status)
	}
	if opened.Warning.Version == "" || opened.Warning.Text == "" {
		t.Fatalf("the review serves warning %+v — a surface with nothing to show would compose "+
			"its own, and the record would name words this installation never wrote",
			opened.Warning)
	}

	// A version the installation does not serve is refused, whatever else the
	// request gets right.
	invented := directed()
	invented["warning_version"] = "whatever-v99"
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		invented, nil, nil); status == http.StatusCreated {
		t.Error("a decision naming a warning version this installation does not serve was " +
			"recorded — the record asserts somebody read words nobody published")
	}

	// And the version the review served is accepted.
	acknowledged := directed()
	acknowledged["warning_version"] = opened.Warning.Version
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		acknowledged, nil, nil); status != http.StatusCreated {
		t.Errorf("the version the review itself served was refused → %d — a director cannot "+
			"acknowledge the warning they were shown", status)
	}
	var recorded string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT warning_version FROM communication_instruction`).Scan(&recorded); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if recorded != opened.Warning.Version {
		t.Errorf("the record names warning %q and the director was shown %q",
			recorded, opened.Warning.Version)
	}
}

// CANCELLING A HELD MESSAGE CLOSES THE REVIEW IT LEFT BEHIND.
//
// A refusal freezes the message and opens a review for it. A rep who then
// cancels has answered that review — not by sending, but by deciding not to.
// Left live, it shows a decider work about a message that is already dead, and
// somebody may record an override for a send that cannot happen.
func TestCancellingAHeldMessageClosesItsReview(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	var held string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil {
		t.Fatalf("finding the held message: %v", err)
	}

	// The state before the cancel, read from the row itself so the audit's
	// before-image is compared against what was really there.
	var state0 string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state FROM communication_review`).Scan(&state0); err != nil {
		t.Fatalf("reading the review: %v", err)
	}

	if status := c.Call(t, "POST", "/v1/scheduled-sends/"+held+"/cancel", AnyMap{}, nil, nil); status >= 400 {
		t.Fatalf("cancelling the held message → %d", status)
	}

	var state string
	var resolved *string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state, resolved_at::text FROM communication_review`).Scan(&state, &resolved); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	if state != "cancelled" {
		t.Errorf("the review reads %q after its message was cancelled, want cancelled — a "+
			"decider is shown work about a message that is already dead", state)
	}
	if resolved == nil {
		t.Error("the review says it is finished and names no moment, which its own shape check " +
			"refuses")
	}
	// CANCELLED, not resolved. Resolved is what a message GOING OUT leaves
	// behind, and a reader asking why somebody received a message must not find
	// one of those about a message nobody got.
	if state == "resolved" {
		t.Error("a cancelled message left a resolved review, which reads as a message that went")
	}
	// AND THE AUDIT SAYS WHAT IT MOVED FROM. The before-image is the half that
	// records the change; writing a fixed value would report every cancelled
	// review as having been the same thing, whatever it actually was.
	var before string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT before->>'status' FROM audit_log
		 WHERE entity_type = 'communication_review' AND action = 'update'
		 ORDER BY occurred_at DESC LIMIT 1`).Scan(&before); err != nil {
		t.Fatalf("reading the audit: %v", err)
	}
	if before == "cancelled" {
		t.Error("the audit says the review moved from cancelled to cancelled — the before-image " +
			"is reading the statement's own result rather than the state it replaced")
	}
	// This refusal is one no evidence can answer — no marketing consent — so
	// the review opened as needs_repair. What matters is that the audit names
	// the state the row actually held rather than a fixed value.
	if before != state0 {
		t.Errorf("the audit says the review moved from %q and the row held %q", before, state0)
	}
}

// A DECISION PAST ITS WINDOW SAYS SO RATHER THAN LOOPING THE CALLER.
//
// The unique key on the review means a standing decision blocks a fresh one. If
// that standing decision has expired, telling the caller "already recorded"
// sends them on to a send that consumption then refuses for a reason nothing
// reported — they press again, are told the same thing, and never learn why.
//
// The instruction's window is aged directly. Waiting out the real validity is
// not a test, and the rule under test is what the DOOR says when a standing
// decision can no longer carry a send.
func TestADecisionPastItsWindowSaysSoRatherThanLoopingTheCaller(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send → %d, want 409", status)
	}
	review := liveReviewID(t, c)

	// A decision is recorded against this review and then left to go stale.
	// Written directly because the route records and sends in one action, and
	// what is under test is the door's answer once the window has closed.
	if _, err := c.Owner.Exec(context.Background(), `
		INSERT INTO communication_instruction
		  (review_id, directed_by, reason_code, explanation, warning_version,
		   acknowledged_at, facts_as_of, valid_until)
		SELECT $1, u.id, 'contractual_necessity', 'decided a while ago', 'override-v1',
		       now() - interval '2 days', now() - interval '2 days', now() - interval '1 day'
		  FROM app_user u WHERE NOT u.is_agent LIMIT 1`, review); err != nil {
		t.Fatalf("recording the stale decision: %v", err)
	}

	var problem struct {
		Code    string `json:"code"`
		Detail  string `json:"detail"`
		Details struct {
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send",
		directed(), nil, &problem)
	if status == http.StatusCreated {
		t.Fatal("a send went out on a decision past its window — the facts it was taken on are " +
			"a day old and nobody has looked at them since")
	}
	// NAMED IN THE FIELD, which is where a typed refusal puts its machine code
	// — the top-level code is the transport's classification and reads
	// validation_error for every field fault.
	named := false
	for _, f := range problem.Details.Errors {
		if f.Code == "decision_not_spendable" {
			named = true
		}
	}
	if !named {
		t.Errorf("the refusal reads %q (%s) and never says the decision cannot be spent — the "+
			"caller presses again forever", problem.Code, problem.Detail)
	}
	// AND IT SAYS WHY in words the caller can act on: taking the decision again
	// is a different move from waiting, and a refusal that did not distinguish
	// them would leave them pressing the same button.
	if !strings.Contains(problem.Detail, "past its window") {
		t.Errorf("the refusal says %q and never says the window has closed", problem.Detail)
	}
}
