// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a rep is left with when the engine refuses their message.
//
// Until this, the answer was 409 consent_not_granted and nothing else. The
// refusal rolled its whole transaction back, so nothing durable said what was
// refused, which recipient it was refused for, or what would change the answer.
// The rep pressed send, read a code, and had nowhere to go.
//
// A review is what the refusal leaves behind now. These tests hold three
// things: the row exists after the send that failed, it names the recipient and
// the reason, and the refusal itself carries the reference so the rep can find
// it without a queue to search.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// reviewRow is what these tests read back.
type reviewRow struct {
	id         string
	state      string
	reasonCode string
	refusals   string
}

// openReviews reads every live review in the installation.
func openReviews(t *testing.T, e *apptest.AppEnv) []reviewRow {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT id::text, state, reason_code, refusals::text
		  FROM communication_review
		 WHERE resolved_at IS NULL
		 ORDER BY opened_at`)
	if err != nil {
		t.Fatalf("reading the reviews: %v", err)
	}
	defer rows.Close()
	var out []reviewRow
	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.id, &r.state, &r.reasonCode, &r.refusals); err != nil {
			t.Fatalf("scanning a review: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the reviews: %v", err)
	}
	return out
}

// A REFUSED SEND LEAVES A REVIEW. This is the slice: the message stopped, and
// there is now something to look at rather than a code in a response body.
func TestARefusedSendLeavesAReviewBehind(t *testing.T) {
	c := setupConsent(t)

	if reviews := openReviews(t, c.AppEnv); len(reviews) != 0 {
		t.Fatalf("%d review(s) before anything was refused, want none", len(reviews))
	}

	status, code := c.send(t, "marketing_email")
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d %q, want 409", status, code)
	}

	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s) after a refused send, want 1 — the rep read a code and the "+
			"installation kept no record of what it refused or for whom", len(reviews))
	}
	// The RECIPIENT is named. A reason code alone says a message was refused
	// and not who for, which is the question the rep is actually asking.
	if !strings.Contains(reviews[0].refusals, "subject@consent.test") {
		t.Errorf("the review names no recipient: %s", reviews[0].refusals)
	}
	if reviews[0].reasonCode == "" {
		t.Error("the review carries no reason code, so nothing says why the send stopped")
	}
}

// THE REFUSAL CARRIES THE REFERENCE. A review nobody can find is a row in a
// table, not a piece of work — the rep has no queue to search and no id to
// quote, so the reference has to travel with the answer they already get.
func TestTheRefusalNamesTheReviewItOpened(t *testing.T) {
	c := setupConsent(t)

	var problem struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
	}, nil, &problem)
	if status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}

	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}
	answer := problem.Message + " " + problem.Detail
	if !strings.Contains(answer, reviews[0].id) {
		t.Errorf("the refusal answered %q and never named review %s — the rep has a row they "+
			"cannot find", answer, reviews[0].id)
	}
}

// A SECOND ATTEMPT AT THE SAME MESSAGE DOES NOT QUEUE A SECOND REVIEW when it
// is bound to one held intent. Two live reviews for one message would put the
// same work in front of somebody twice.
//
// This slice binds no intent yet — an immediate send has no held row — so each
// attempt is its own refusal and its own review. That is the honest behaviour
// for now and this test pins it, so the slice that starts holding intents has
// to come here and say the rule changed.
func TestEachUnboundAttemptRecordsItsOwnRefusal(t *testing.T) {
	c := setupConsent(t)

	for range 2 {
		if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
			t.Fatalf("marketing send with no grant → %d, want 409", status)
		}
	}
	if reviews := openReviews(t, c.AppEnv); len(reviews) != 2 {
		t.Errorf("%d review(s) after two refused attempts, want 2 — an unbound refusal has no "+
			"intent to supersede against, so each attempt is its own record", len(reviews))
	}
}

// AN ERASURE CLEARS THE SUBJECT FROM A REVIEW. The row holds the addresses a
// message was refused for, which is exactly the material Art. 17 destroys — and
// unlike a decision beside it, a review is not accountability evidence: it
// records a message that never went, so there is nothing to answer for.
//
// The row SURVIVES with its refusals emptied rather than being deleted. The
// work may still be in front of a human, and a row vanishing under them leaves
// a queue pointing at nothing.
func TestErasingASubjectClearsThemFromARefusedSendReview(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 || !strings.Contains(reviews[0].refusals, "subject@consent.test") {
		t.Fatalf("no review naming the subject to erase from: %+v", reviews)
	}

	var personID string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT p.id::text FROM person p
		  JOIN person_email e ON e.person_id = p.id
		 WHERE lower(e.email) = 'subject@consent.test'`).Scan(&personID); err != nil {
		t.Fatalf("finding the subject: %v", err)
	}
	// THROUGH THE RIGHTS CASE, which is how an erasure actually happens here: a
	// case is opened naming the subject and fulfilling it runs the eraser. That
	// is the production route, so this proves the review is cleared by the path
	// an operator takes rather than by one a test invented.
	var opened struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": personID,
		"due_at": "2027-01-01T00:00:00Z",
	}, nil, &opened); status != http.StatusCreated {
		t.Fatalf("opening the erasure case → %d", status)
	}
	if status := c.Call(t, "PATCH", "/v1/data-subject-requests/"+opened.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased on request",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure → %d", status)
	}

	after := openReviews(t, c.AppEnv)
	if len(after) != 1 {
		t.Fatalf("%d review(s) after the erasure, want the row to survive with its refusals "+
			"emptied — a queue pointing at a deleted row shows a human nothing", len(after))
	}
	if strings.Contains(after[0].refusals, "subject@consent.test") {
		t.Errorf("the review still names the erased subject's address: %s", after[0].refusals)
	}
}

// A REFERENCE NOBODY CAN RESOLVE IS BARELY BETTER THAN AN ERROR CODE. The
// refusal answers a review id and, until this endpoint existed, nothing could
// open it: the row was written, the rep was handed the id, and there was no
// door.
func TestTheRepCanOpenTheReviewTheirRefusalNamed(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}

	var opened struct {
		ID         string `json:"id"`
		State      string `json:"state"`
		ReasonCode string `json:"reason_code"`
		Refusals   []struct {
			Address    string `json:"address"`
			ReasonCode string `json:"reason_code"`
		} `json:"refusals"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+reviews[0].id,
		nil, nil, &opened); status != http.StatusOK {
		t.Fatalf("opening the review the refusal named → %d, want 200", status)
	}
	if opened.ID != reviews[0].id {
		t.Errorf("opened review %q, want %q", opened.ID, reviews[0].id)
	}
	if len(opened.Refusals) != 1 {
		t.Fatalf("%d refusal(s) in the answer, want 1 — the rep is told a send was refused "+
			"and not who for", len(opened.Refusals))
	}
	if opened.Refusals[0].Address != "subject@consent.test" {
		t.Errorf("the answer names %q, want the recipient the send was refused for",
			opened.Refusals[0].Address)
	}
	if opened.ReasonCode == "" || opened.State == "" {
		t.Errorf("the answer carries state=%q reason=%q, want both",
			opened.State, opened.ReasonCode)
	}
}

// A REVIEW BELONGING TO SOMEBODY ELSE IS NOT FOUND, not forbidden. The row
// names the recipients of another person's message and why each was refused,
// which is a fact about those people — and "forbidden" would confirm the id
// exists, which is itself a disclosure about a message the caller may not see.
func TestAnotherSeatsReviewIsNotFound(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	reviews := openReviews(t, c.AppEnv)
	if len(reviews) != 1 {
		t.Fatalf("%d review(s), want 1", len(reviews))
	}

	// Reassigned to a seat this caller is not: the same shape as another rep's
	// refusal, without needing a second logged-in session to produce one.
	var other string
	if err := c.Owner.QueryRow(context.Background(), `
		INSERT INTO app_user (email, display_name)
		VALUES ('other-' || gen_random_uuid() || '@consent.test', 'Other Seat')
		RETURNING id::text`).Scan(&other); err != nil {
		t.Fatalf("seeding another seat: %v", err)
	}
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_review SET initiated_by = $1 WHERE id = $2`,
		other, reviews[0].id); err != nil {
		t.Fatalf("reassigning the review: %v", err)
	}

	if status := c.Call(t, "GET", "/v1/communication-reviews/"+reviews[0].id,
		nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("another seat's review → %d, want 404 — a 403 would confirm the id exists, "+
			"which is a disclosure about a message this caller may not see", status)
	}
}
