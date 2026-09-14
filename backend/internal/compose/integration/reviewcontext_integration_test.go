// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Answering a refusal by saying what happened away from the system.
//
// The engine refuses a send to somebody it has no evidence about, and it is
// right to: nothing on the record connects this workspace to that contact. But
// the record is not the world. A customer rang and asked for a quote; somebody
// took a card at a stand. The rep who was there is the only place that fact
// exists, and the review that refused them is where they put it on the record.
//
// WHY THESE TESTS EXIST IN THIS SHAPE. An earlier version of this endpoint
// shipped and was reverted, because it wrote the statement to a table nothing
// reads back as evidence: it answered success and the very next send was
// refused identically. The rep typed a sentence into a box and nothing changed.
//
// So the assertion that matters here is not "the endpoint answered 200". It is
// that the SAME SEND, retried, now goes. Every test below ends on that.

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// coldContact is somebody this workspace has no evidence about: no inbound, no
// deal, no meeting. The fixture's own subject has an inbound on file, which is
// exactly the evidence these tests need to be missing.
type coldContact struct {
	contactID string
	address   string
}

func aColdContact(t *testing.T, c *consentEnv, address string) coldContact {
	t.Helper()
	var contact struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Cold Contact",
		"emails":    []AnyMap{{"email": address}},
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("creating the cold contact → %d", status)
	}
	return coldContact{contactID: contact.ID, address: address}
}

// sendCold attempts the account-started send that has nothing behind it, and
// answers the problem code so a caller can assert WHY it was refused.
func sendCold(t *testing.T, c *consentEnv, who coldContact) (int, string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/emails", AnyMap{
		"subject":         "About your order",
		"body":            "Here is the quote you asked for.",
		"to":              []string{who.address},
		"links":           []AnyMap{{"entity_type": "contact", "entity_id": who.contactID}},
		"consent_purpose": "business_correspondence",
	}, nil, &problem)
	return status, problem.Code
}

// liveReviewFor reads the review a refusal opened for one message.
func liveReviewFor(t *testing.T, c *consentEnv) string {
	t.Helper()
	var id string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM communication_review WHERE resolved_at IS NULL
		  ORDER BY opened_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	return id
}

// THE STATEMENT CHANGES THE ANSWER, WHICH IS THE WHOLE FEATURE.
//
// This is the test the reverted attempt did not have. It does not check that
// the endpoint returned 200; it retries the send that was refused and requires
// it to go.
func TestAFirsthandStatementLetsTheRefusedMessageGo(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "rang.up@cold.test")

	status, code := sendCold(t, c, who)
	if status != http.StatusConflict {
		t.Fatalf("a send to somebody with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	rang := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "Rang about the March order and asked me to send the quote by email.",
		"occurred_at": rang,
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("recording the statement → %d, want 200", status)
	}

	// THE SAME SEND, RETRIED. A rep who answers the question the engine asked
	// has to be able to press send and have it go.
	if status, code := sendCold(t, c, who); status != http.StatusAccepted {
		t.Fatalf("the same send after the statement → %d %q, want 202 — the rep answered the "+
			"question and the message is still refused, which is the defect this endpoint was "+
			"reverted for", status, code)
	}
}

// A STATEMENT NAMES THE CONTACT IT IS ABOUT.
//
// The reverted attempt copied one sentence onto every refused recipient, so a
// note about a phone call with one contact became recorded evidence about
// others who had nothing to do with it. The subject is named in the body, and
// a subject this review was not refused for is not found.
func TestAStatementAboutSomebodyElseIsNotFound(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "named@cold.test")
	stranger := aColdContact(t, c, "stranger@cold.test")

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("a send to somebody with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  stranger.contactID,
		"kind":        "requested_by_subject",
		"note":        "Rang me about something else entirely.",
		"occurred_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}, nil, nil); status != http.StatusNotFound {
		t.Fatalf("a statement about somebody this review never named → %d, want 404", status)
	}

	// AND NOTHING WAS WRITTEN ABOUT THEM. A 404 that had already recorded the
	// evidence would be the same defect wearing a different status code.
	var events int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`,
		stranger.contactID).Scan(&events); err != nil {
		t.Fatalf("counting what was recorded about the stranger: %v", err)
	}
	if events != 0 {
		t.Errorf("%d qualifying event(s) recorded about a contact this review never named", events)
	}
}

// A STATEMENT DOES NOT OVERRIDE THE SUBJECT'S OWN DECISION.
//
// A qualifying event settles whether ordinary correspondence is lawful at all.
// It does not touch a stop the contact themselves recorded — and an endpoint
// that accepted the sentence anyway would write a row that changes nothing,
// which is the failure mode this whole slice exists to end.
//
// The refusal names what is really in the way, because the rep is about to go
// looking for the next move.
func TestAStatementCannotAnswerAStopTheSubjectRecorded(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "asked.us.to.stop@cold.test")

	if status := c.Call(t, "POST", "/v1/contacts/"+who.contactID+"/consent/suppress", AnyMap{
		"kind":   "subject_request",
		"reason": "Asked us not to contact them again.",
	}, nil, nil); status >= 400 {
		t.Fatalf("recording the subject's stop → %d", status)
	}

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("a send to somebody who asked us to stop → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	var problem struct {
		Details struct {
			Errors []struct {
				Field   string `json:"field"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"details"`
	}
	status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "But they rang me and asked for this one.",
		"occurred_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}, nil, &problem)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a statement against a recorded stop → %d, want 422 — accepting it would write "+
			"a row that changes nothing and tell the rep it worked", status)
	}
	if len(problem.Details.Errors) == 0 {
		t.Fatal("the refusal names no field, so a screen cannot show the rep what is in the way")
	}
	if problem.Details.Errors[0].Code != "context_does_not_answer_this" {
		t.Errorf("the refusal is coded %q, want context_does_not_answer_this",
			problem.Details.Errors[0].Code)
	}

	// AND THE STOP IS UNTOUCHED. The message is still refused for the same
	// reason it was before anybody typed anything.
	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Errorf("the send after the refused statement → %d %q, want 409 still", status, code)
	}
}

// A STATEMENT THE VERDICT WOULD IGNORE IS REFUSED RATHER THAN ACCEPTED.
//
// The verdict reads recorded exchanges only inside the reply window. A
// statement about something older is written, stored, and never read — so the
// endpoint would report success and the very next send would be refused
// identically. That is this slice's own failure mode, arriving on the date axis
// instead of the table axis, and it is what Codex found on the second round.
func TestAnExchangeTooOldToCountIsRefusedNotAccepted(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "long.ago@cold.test")

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("a send to somebody with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	// Well past the 365-day default window.
	longAgo := time.Now().Add(-400 * 24 * time.Hour).UTC().Format(time.RFC3339)
	status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "Rang me, but it was a very long time ago.",
		"occurred_at": longAgo,
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a statement older than the window → %d, want 422 — accepting it writes a row "+
			"the verdict never reads and tells the rep it worked", status)
	}

	// AND NOTHING WAS WRITTEN. A 422 that had already recorded the row would
	// leave dead evidence behind and shadow nothing useful.
	var events int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`,
		who.contactID).Scan(&events); err != nil {
		t.Fatalf("counting what was recorded: %v", err)
	}
	if events != 0 {
		t.Errorf("%d qualifying event(s) written for a statement that was refused", events)
	}
}

// A STOP RECORDED AFTER THE REVIEW OPENED IS SEEN.
//
// The refusals on the review row are a snapshot taken when the send was
// refused. A contact who asks us to stop AFTER that is not in them, so a review
// still reading "no evidence" can belong to somebody who has since said no —
// and accepting a statement there reports success about a message that will be
// refused anyway.
func TestAStopRecordedAfterTheReviewOpenedIsStillSeen(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "changed.their.mind@cold.test")

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("a send to somebody with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	// The contact asks us to stop, AFTER the review was opened and its refusals
	// frozen onto the row.
	if status := c.Call(t, "POST", "/v1/contacts/"+who.contactID+"/consent/suppress", AnyMap{
		"kind":   "subject_request",
		"reason": "Rang back and asked us not to contact them.",
	}, nil, nil); status >= 400 {
		t.Fatalf("recording the stop → %d", status)
	}

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "They did ask me for this earlier though.",
		"occurred_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a statement about somebody who has since asked us to stop → %d, want 422 — the "+
			"review's refusals are a snapshot and do not know about the stop", status)
	}
}

// A REVIEW THAT IS OVER TAKES NO STATEMENT.
//
// Cancelled and resolved reviews stay READABLE on purpose, so a queue does not
// develop holes. Staying WRITABLE is a different thing: a rep could answer a
// review whose message was cancelled weeks ago and write evidence that then
// authorizes some unrelated future send.
func TestAReviewThatIsOverTakesNoStatement(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "cancelled@cold.test")

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("a send to somebody with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	var held string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil {
		t.Fatalf("finding the held message: %v", err)
	}
	if status := c.Call(t, "POST", "/v1/scheduled-sends/"+held+"/cancel",
		AnyMap{}, nil, nil); status >= 400 {
		t.Fatalf("cancelling the message → %d", status)
	}

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "They rang me about it.",
		"occurred_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a statement on a cancelled review → %d, want 422", status)
	}

	var events int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`,
		who.contactID).Scan(&events); err != nil {
		t.Fatalf("counting what was recorded: %v", err)
	}
	if events != 0 {
		t.Errorf("%d qualifying event(s) written from a review whose message was cancelled", events)
	}
}

// A MARKETING OBJECTION DOES NOT BLOCK EVIDENCE FOR AN ORDINARY LETTER.
//
// This pins the correction to a round-one fix that was itself too blunt. The
// first spelling of the stop check refused a statement whenever ANY live
// suppression existed, which turned somebody who had opted out of the
// newsletter into a contact a rep could not record a phone call about — while
// the send path would have let the ordinary letter through, because a marketing
// objection binds marketing only (authorizesuppression.go, suppressionBinds).
//
// The check now asks whether a stop binds THIS message's category, through the
// engine's own rule rather than a second copy of it.
func TestAMarketingObjectionDoesNotBlockEvidenceForAnOrdinaryLetter(t *testing.T) {
	c := setupConsent(t)
	who := aColdContact(t, c, "no.newsletter@cold.test")

	if status := c.Call(t, "POST", "/v1/contacts/"+who.contactID+"/consent/suppress", AnyMap{
		"kind":   "marketing_objection",
		"reason": "Asked us to stop sending the newsletter.",
	}, nil, nil); status >= 400 {
		t.Fatalf("recording the marketing objection → %d", status)
	}

	if status, code := sendCold(t, c, who); status != http.StatusConflict {
		t.Fatalf("an ordinary letter with no evidence → %d %q, want 409", status, code)
	}
	review := liveReviewFor(t, c)

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/context", AnyMap{
		"subject_id":  who.contactID,
		"kind":        "requested_by_subject",
		"note":        "Rang about the March order and asked me to send the quote.",
		"occurred_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("a statement about somebody who only stopped MARKETING → %d, want 200 — their "+
			"newsletter opt-out says nothing about whether they rang up asking for a quote", status)
	}

	// AND THE LETTER GOES, which is the half that proves the statement was not
	// merely accepted but actually counted.
	if status, code := sendCold(t, c, who); status != http.StatusAccepted {
		t.Fatalf("the ordinary letter after the statement → %d %q, want 202", status, code)
	}

	// THE OBJECTION IS UNTOUCHED. Recording correspondence evidence must not
	// quietly restore marketing.
	var live int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression
		  WHERE contact_id = $1 AND kind = 'marketing_objection' AND revoked_at IS NULL`,
		who.contactID).Scan(&live); err != nil {
		t.Fatalf("reading the objection: %v", err)
	}
	if live != 1 {
		t.Errorf("the marketing objection reads %d live rows, want 1 — recording correspondence "+
			"evidence must not lift a marketing stop", live)
	}
}
