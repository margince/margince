// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Subscribing again, from the subject's own preference page.
//
// A withdrawal was never meant to be permanent. The mint guard that refuses
// somebody ELSE restarting the conversation says so in its own comment: "a
// withdrawal is not permanent for the SUBJECT — they may re-subscribe through
// the preference centre, which is their own mailbox and their own choice."
//
// That sentence was not true. Two things stood in the way. The preference
// centre wrote `granted` on the row for a double-opt-in purpose with no
// confirmation round trip, and the send gate refuses an unconfirmed grant — so
// the subject pressed subscribe, the page said yes, and nothing ever arrived.
// And the guard itself refused their own ask along with everybody else's.
//
// Their only remaining route was to ask somebody at the company to restart it,
// which is exactly what the guard refuses, and rightly.

import (
	"context"
	"net/http"
	"testing"
)

// choiceOutcome is what the save reports back about one purpose.
type choiceOutcome struct {
	PurposeKey string `json:"purpose_key"`
	Reason     string `json:"reason"`
}

// savePreference puts one choice and returns whatever the page is told about it.
func savePreference(t *testing.T, c *consentEnv, token, key, state string) []choiceOutcome {
	t.Helper()
	var out struct {
		Refused []choiceOutcome `json:"refused"`
	}
	if s := publicCall(t, c.AppEnv, "PUT", "/v1/public/preferences/"+token, AnyMap{
		"choices": []AnyMap{{
			"purpose_key": key, "state": state,
			"wording": "Yes, keep me posted.",
		}},
	}, nil, &out); s != http.StatusOK {
		t.Fatalf("saving %s=%s → %d", key, state, s)
	}
	return out.Refused
}

// doiPurposeKey is the marketing purpose these tests subscribe to.
const doiPurposeKey = "product_news"

// createDOIPurpose creates a marketing purpose that needs the round trip.
func createDOIPurpose(t *testing.T, c *consentEnv) string {
	t.Helper()
	var purpose struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/consent-purposes", AnyMap{
		"key": doiPurposeKey, "label": "Product news", "requires_double_opt_in": true,
	}, nil, &purpose); status != http.StatusCreated {
		t.Fatalf("create %s → %d", doiPurposeKey, status)
	}
	return purpose.ID
}

// confirmTokensMinted counts the live confirm links for the fixture's subject.
func confirmTokensMinted(t *testing.T, c *consentEnv) int {
	t.Helper()
	var n int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE consumed_at IS NULL`).Scan(&n); err != nil {
		t.Fatalf("counting confirm links: %v", err)
	}
	return n
}

// PRESSING SUBSCRIBE SENDS A CONFIRMATION RATHER THAN WRITING A DEAD GRANT.
func TestResubscribingSendsAConfirmationInsteadOfADeadGrant(t *testing.T) {
	c := setupConsent(t)
	createDOIPurpose(t, c)
	// A mail has to go out for the subject to hold a preference link at all,
	// and it goes under a purpose that needs no round trip.
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	// The subject is on the preference page holding their own token.
	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	before := confirmTokensMinted(t, c)
	outcomes := savePreference(t, c, token, "product_news", "granted")

	// THE PAGE IS TOLD THE ANSWER IS PENDING. A save reported as plain success
	// leaves somebody expecting mail that will not come until they click.
	if len(outcomes) != 1 || outcomes[0].Reason != "confirmation_sent" {
		t.Fatalf("the save reported %+v, want one confirmation_sent — the subject has to be told "+
			"their subscribe is waiting on a link, not that it is done", outcomes)
	}

	// A LINK WENT OUT.
	if after := confirmTokensMinted(t, c); after != before+1 {
		t.Errorf("%d confirm link(s) before and %d after, want one more — the subject was told a "+
			"confirmation is coming and none was minted", before, after)
	}

	// AND NO GRANT WAS WRITTEN. A row saying the subject consented, at a moment
	// they had only asked to be asked, is believed by every reader of
	// contact_consent — and refused by the send gate anyway.
	var state string
	err := c.Owner.QueryRow(context.Background(), `
		SELECT pc.state FROM contact_consent pc
		  JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.contact_id = $1 AND cp.key = 'product_news'`, c.contactID).Scan(&state)
	if err == nil && state == "granted" {
		t.Error("the page wrote `granted` before the subject confirmed — the send gate refuses " +
			"that row anyway, so it records a consent that does nothing")
	}
}

// THE SUBJECT'S OWN ASK IS NOT A RE-SOLICITATION.
//
// The mint guard refuses asking again about a purpose somebody took back. It
// has to: an operator pressing the verb on a withdrawn contact is restarting a
// conversation that contact ended. But it refused the SUBJECT's own ask too,
// which made a withdrawal permanent — the one thing the guard's own comment
// says it does not do.
func TestASubjectMayAskAgainAfterWithdrawing(t *testing.T) {
	c := setupConsent(t)
	createDOIPurpose(t, c)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	// They withdraw first, which is what arms the guard.
	savePreference(t, c, token, "product_news", "withdrawn")
	var state string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT pc.state FROM contact_consent pc
		  JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.contact_id = $1 AND cp.key = 'product_news'`, c.contactID).Scan(&state); err != nil {
		t.Fatalf("reading the withdrawal: %v", err)
	}
	if state != "withdrawn" {
		t.Fatalf("the purpose reads %q after a withdrawal, so the guard this test is about is "+
			"not armed", state)
	}

	before := confirmTokensMinted(t, c)
	outcomes := savePreference(t, c, token, "product_news", "granted")
	if len(outcomes) != 1 || outcomes[0].Reason != "confirmation_sent" {
		t.Fatalf("asking again after withdrawing reported %+v, want confirmation_sent — the "+
			"subject can no longer subscribe themselves and the guard refuses anybody doing it "+
			"for them, so the withdrawal is permanent", outcomes)
	}
	if after := confirmTokensMinted(t, c); after != before+1 {
		t.Errorf("%d confirm link(s) before and %d after, want one more", before, after)
	}
}

// AN OPERATOR STILL CANNOT RESTART IT.
//
// Without this the fix above would read as a fix and be a hole: lifting the
// guard for everybody would let a rep press the double-opt-in verb on somebody
// who unsubscribed, which is what it was built to stop.
func TestAnOperatorStillCannotAskAgainAfterAWithdrawal(t *testing.T) {
	c := setupConsent(t)
	purposeID := createDOIPurpose(t, c)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))
	savePreference(t, c, token, "product_news", "withdrawn")

	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/double-opt-in",
		AnyMap{"purpose_id": purposeID}, nil, nil); status < 400 {
		t.Errorf("an operator restarted a conversation the subject ended → %d, want a refusal",
			status)
	}
}

// A PURPOSE THAT NEEDS NO ROUND TRIP IS UNCHANGED.
//
// Sending a confirmation nobody asked for would be a mail this contact did not
// need, and the grant is effective the moment it is written.
func TestANonDoubleOptInSubscribeStillTakesEffectAtOnce(t *testing.T) {
	c := setupConsent(t)
	newsletter := createNewsletterPurpose(t, c)
	grantPurpose(t, c, newsletter)

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	before := confirmTokensMinted(t, c)
	if outcomes := savePreference(t, c, token, "newsletter", "granted"); len(outcomes) != 0 {
		t.Fatalf("a purpose needing no round trip reported %+v, want a plain save", outcomes)
	}
	if after := confirmTokensMinted(t, c); after != before {
		t.Errorf("%d confirm link(s) before and %d after — a confirmation was mailed for a "+
			"purpose that needs none", before, after)
	}

	var state string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT pc.state FROM contact_consent pc
		  JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.contact_id = $1 AND cp.key = 'newsletter'`, c.contactID).Scan(&state); err != nil {
		t.Fatalf("reading the grant: %v", err)
	}
	if state != "granted" {
		t.Errorf("the purpose reads %q, want granted — this one takes effect when written", state)
	}
}

// THE ROUND TRIP ACTUALLY COMPLETES, which is the whole feature.
//
// Everything above proves a link is minted. This proves that spending it leaves
// a grant the SEND GATE honours — which is what the subject was promised when
// they pressed subscribe.
//
// It is the case that nearly shipped broken. An unconfirmed `granted` row
// already sat there, and recordAdmittedTx treated the confirmation as an
// idempotent re-assertion of a state the row already claimed: no proof row, no
// confirmation moment, and the message still refused. The repair would have
// been a no-op against exactly the shape it was built to repair.
func TestSpendingTheResubscribeLinkLeavesAGrantTheGateHonours(t *testing.T) {
	c := setupConsent(t)
	createDOIPurpose(t, c)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	if outcomes := savePreference(t, c, token, "product_news", "granted"); len(outcomes) != 1 {
		t.Fatalf("the subscribe reported %+v, want one confirmation_sent", outcomes)
	}

	// The subject opens the link that was mailed to them and confirms.
	confirm := confirmLinkToken(t, c.AppEnv)
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+confirm, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, keep me posted.",
	}, nil, nil); s != http.StatusOK {
		t.Fatalf("spending the confirmation link → %d, want 200", s)
	}

	// THE PROOF CARRIES BOTH HALVES the gate asks for: a confirmation moment
	// and the mailbox proof that names what it rests on. A row with one and not
	// the other is refused, and the subject would be back where they started.
	var confirmed, trigger *string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT ce.double_opt_in_confirmed_at::text, ce.issuance_trigger
		  FROM consent_event ce JOIN consent_purpose cp ON cp.id = ce.purpose_id
		 WHERE ce.contact_id = $1 AND cp.key = 'product_news' AND ce.new_state = 'granted'
		 ORDER BY ce.captured_at DESC LIMIT 1`, c.contactID).Scan(&confirmed, &trigger); err != nil {
		t.Fatalf("reading the proof the confirmation should have written: %v", err)
	}
	if confirmed == nil {
		t.Error("the confirmation wrote no round-trip moment, so the send gate still refuses " +
			"every message — the subject confirmed and nothing changed")
	}
	if trigger == nil {
		t.Error("the proof names no mailbox proof, which the gate also requires")
	}
}

// A GRANT ALREADY SITTING THERE UNCONFIRMED IS REPAIRED, NOT SHRUGGED OFF.
//
// This is the row shape the defect leaves behind and the one the whole slice
// exists to repair: state `granted`, no confirmation moment, and every send
// refused. It is what the preference centre wrote before this change, and it is
// on live installations now.
//
// recordAdmittedTx treated the confirmation as an idempotent re-assertion of a
// state the row already claimed — no proof row, no confirmation recorded — so
// spending the link changed nothing and the subject was back where they
// started. The tests above cannot see it, because the new path never writes
// that row; this one puts it there first, the way the old code did.
func TestAnUnconfirmedGrantIsRepairedByTheConfirmation(t *testing.T) {
	c := setupConsent(t)
	purposeID := createDOIPurpose(t, c)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	// The damaged row, written the way the preference centre used to: granted,
	// with nothing evidencing a round trip.
	if _, err := c.Owner.Exec(context.Background(), `
		INSERT INTO contact_consent (contact_id, purpose_id, state, lawful_basis, captured_at)
		VALUES ($1, $2, 'granted', 'consent', now())
		ON CONFLICT (contact_id, purpose_id) DO UPDATE SET state = 'granted'`,
		c.contactID, purposeID); err != nil {
		t.Fatalf("seeding the unconfirmed grant: %v", err)
	}

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	// The subject presses subscribe. The row already says granted, so nothing
	// about the STATE changes — what they need is the confirmation.
	if outcomes := savePreference(t, c, token, "product_news", "granted"); len(outcomes) != 1 ||
		outcomes[0].Reason != "confirmation_sent" {
		t.Fatalf("subscribing over an unconfirmed grant reported %+v, want confirmation_sent — "+
			"the row reads granted and the send gate refuses it, so there is everything to do",
			outcomes)
	}

	confirm := confirmLinkToken(t, c.AppEnv)
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+confirm, AnyMap{
		"marketing_choice": "granted", "marketing_wording": "Yes, keep me posted.",
	}, nil, nil); s != http.StatusOK {
		t.Fatalf("spending the confirmation link → %d", s)
	}

	var confirmed, trigger *string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT ce.double_opt_in_confirmed_at::text, ce.issuance_trigger
		  FROM consent_event ce
		 WHERE ce.contact_id = $1 AND ce.purpose_id = $2 AND ce.new_state = 'granted'
		 ORDER BY ce.captured_at DESC LIMIT 1`, c.contactID, purposeID).Scan(&confirmed, &trigger); err != nil {
		t.Fatalf("the confirmation wrote no proof row at all over the unconfirmed grant: %v", err)
	}
	if confirmed == nil || trigger == nil {
		t.Errorf("the confirmation was treated as an idempotent re-assertion of a state the row "+
			"already claimed, so it wrote no round trip (confirmed=%v trigger=%v) — the send gate "+
			"still refuses and the repair was a no-op", confirmed, trigger)
	}
}

// AN ALREADY-CONFIRMED SUBSCRIBE MAILS NOTHING.
//
// Without this the short-circuit could be deleted and every test above would
// still pass, while a subject re-ticking a box they already hold would be sent
// a confirmation for a decision they already made.
func TestReTickingAConfirmedSubscriptionMailsNothing(t *testing.T) {
	c := setupConsent(t)
	createDOIPurpose(t, c)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	sendMarketing(t, c.AppEnv, c.activityID, "newsletter", "", "")
	token := manageTokenFromLink(t, manageLinkIn(t, transmittedBody(t, c)))

	savePreference(t, c, token, "product_news", "granted")
	confirm := confirmLinkToken(t, c.AppEnv)
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+confirm, AnyMap{
		"marketing_choice": "granted", "marketing_wording": "Yes.",
	}, nil, nil); s != http.StatusOK {
		t.Fatalf("confirming → %d", s)
	}

	before := confirmTokensMinted(t, c)
	if outcomes := savePreference(t, c, token, "product_news", "granted"); len(outcomes) != 0 {
		t.Errorf("re-ticking a confirmed subscription reported %+v, want a quiet no-op", outcomes)
	}
	if after := confirmTokensMinted(t, c); after != before {
		t.Errorf("%d confirm link(s) before and %d after — somebody was mailed a confirmation "+
			"for a decision they had already made", before, after)
	}
}
