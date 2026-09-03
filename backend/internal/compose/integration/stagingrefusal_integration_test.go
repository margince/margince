// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A recipient who said stop is refused at the door, on BOTH doors.
//
// The staging decision is written for every queued message, and the census gate
// beside it proves the call happens. A call is not an enforcement: the gate
// sees that AuthorizeStagingTx was invoked and cannot see whether anybody read
// its answer. Deleting the refusal from both doors left the whole unit lane
// green, which is what these two tests are here to stop.
//
// What a missing refusal costs is not abstract. The send stages, the activity
// commits attesting that this installation corresponded with the person, and a
// message goes to somebody carrying an Art. 21 objection. The rep learns
// nothing until the transmit gate parks the row in an operator lane days
// later, by which time the outbound activity is on the timeline as evidence of
// a conversation that should never have happened.
//
// One test per door, and deliberately not one table-driven test over both: the
// mail path and the channel path are two implementations of staging, and a
// mutation check has to be able to revert one without the other going quiet.

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// suppressPerson records an Art. 21 marketing objection against a person.
//
// Written directly rather than through a store method because there is no
// production writer for communication_suppression yet — the unsubscribe path
// that will own it lands with the preference centre. The row shape is the one
// liveSuppression reads, and when that writer arrives this helper is what
// should be repointed at it.
func suppressPerson(t *testing.T, e *apptest.AppEnv, personID string) {
	t.Helper()
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO communication_suppression (person_id, kind, source, captured_by)
			VALUES ($1, 'marketing_objection', 'test', $2)`, personID, "test")
		return err
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
}

// The mail door. A staged send to an objecting recipient is refused with the
// whole transaction, so no delivery row survives it.
func TestAMailSendToAnObjectingRecipientStagesNothing(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	suppressPerson(t, p.AppEnv, p.personID)

	status, code, _ := p.send(t)

	if status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("mail to an objecting recipient → %d %q, want 409 consent_not_granted", status, code)
	}
	if n := p.stagedDeliveries(t); n != 0 {
		t.Fatalf("%d deliveries staged behind a refused send, want 0 — "+
			"a refusal that leaves a row behind still queued the message", n)
	}
}

// The channel door, which is a second implementation of staging rather than a
// variant of the mail one. A fix to the mail path does not reach it, so it is
// asserted separately.
func TestAChannelSendToAnObjectingRecipientStagesNothing(t *testing.T) {
	c := setupChannelSend(t)
	c.grantConsent(t, "transactional")
	suppressPerson(t, c.AppEnv, c.personID)

	status, code, _ := c.sendReply(t, "transactional", "Yes — shipping Monday.", nil)

	if status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("channel reply to an objecting recipient → %d %q, want 409 consent_not_granted", status, code)
	}
	c.assertNoOutboundEffect(t, "a send to an objecting recipient")
}

// stagingDecisions reads back what the engine wrote for one delivery at the
// staging phase: the recipient it decided about, its verdict and its actor.
func (p *preflightEnv) stagingDecisions(t *testing.T, deliveryID ids.UUID) []stagedDecision {
	t.Helper()
	var rows []stagedDecision
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		found, err := tx.Query(context.Background(), `
			SELECT recipient_address, verdict, reason_code, attempt, actor
			  FROM communication_decision
			 WHERE delivery_id = $1 AND phase = 'staging'
			 ORDER BY recipient_address`, deliveryID)
		if err != nil {
			return err
		}
		defer found.Close()
		for found.Next() {
			var d stagedDecision
			if err := found.Scan(&d.address, &d.verdict, &d.reason, &d.attempt, &d.actor); err != nil {
				return err
			}
			rows = append(rows, d)
		}
		return found.Err()
	}); err != nil {
		t.Fatalf("reading the staging decisions: %v", err)
	}
	return rows
}

type stagedDecision struct {
	address, verdict, reason, actor string
	attempt                         int
}

// The allow path writes the row, and the row says who decided.
//
// The refusal tests above prove the engine's answer is acted on; this proves
// the answer is recorded. Without it the whole table could stay empty and every
// other test in this file would still pass — a refusal that leaves no trail is
// exactly as unanswerable later as no refusal at all.
func TestAnAllowedSendRecordsItsStagingDecision(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	sent := p.sendExpectingAcceptance(t, "transactional", "Re: Inbound question", "As discussed.")
	deliveryID, _ := p.deliveryFor(t, sent)

	rows := p.stagingDecisions(t, deliveryID)
	if len(rows) != 1 {
		t.Fatalf("%d staging decisions for a one-recipient send, want 1", len(rows))
	}
	d := rows[0]
	if d.address != "buyer@preflight.test" {
		t.Errorf("decision recorded for %q, want the recipient the message went to", d.address)
	}
	if d.verdict != "allow" {
		t.Errorf("verdict = %q, want allow for a consented recipient", d.verdict)
	}
	// Zero means "before any attempt": every transmit row that follows carries
	// the attempt it belonged to, so a staging row sharing that numbering would
	// be indistinguishable from the first transmit.
	if d.attempt != 0 {
		t.Errorf("attempt = %d, want 0 — a staging decision precedes every attempt", d.attempt)
	}
	// From the authenticated principal, never the request body: the row is
	// evidence of who authorized the message.
	if d.actor == "" {
		t.Error("the decision names no actor — a proof row nobody can be held to is not proof")
	}
}

// sendExpectingAcceptanceClaiming sends while naming a category, and expects
// the send to be accepted. It is the accepting twin of sendClaiming below.
func (p *preflightEnv) sendExpectingAcceptanceClaiming(t *testing.T, context string) ids.UUID {
	t.Helper()
	var sent struct {
		ID string `json:"id"`
	}
	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "As discussed.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"communication_context": context,
	}, nil, &sent)
	if status != http.StatusAccepted {
		t.Fatalf("send-email claiming %q → %d, want 202", context, status)
	}
	id, err := ids.Parse(sent.ID)
	if err != nil {
		t.Fatalf("accepted send returned no activity id: %v", err)
	}
	return id
}

// sendClaiming posts a send naming a communication context, and returns the
// status plus the problem code.
func (p *preflightEnv) sendClaiming(t *testing.T, context string) (status int, code string) {
	t.Helper()
	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	status = p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"communication_context": context,
	}, nil, &problem)
	if errs := problem.Details.Errors; len(errs) > 0 {
		return status, errs[0].Code
	}
	return status, problem.Code
}

// A rep may say what their own message is about, and the claim is recorded
// beside the category the engine resolved — so a claim the evidence does not
// support is visible later rather than silently honoured or silently dropped.
func TestAClaimedContextReachesTheDecision(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	sent := p.sendExpectingAcceptanceClaiming(t, "reply_to_inbound")
	deliveryID, _ := p.deliveryFor(t, sent)

	var requested *string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT requested_category FROM communication_decision
			  WHERE delivery_id = $1 AND phase = 'staging'`, deliveryID).Scan(&requested)
	}); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if requested == nil || *requested != "reply_to_inbound" {
		t.Fatalf("requested_category = %v, want the category the caller claimed", requested)
	}
}

// The five categories reserved for controller mail are refused at the send
// door. A send able to claim one could dress marketing as a security warning
// and pass a suppression that exists to stop exactly that.
func TestASendCannotClaimAControllerCategory(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	status, code := p.sendClaiming(t, "security_notice")

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a send claiming security_notice → %d, want 422", status)
	}
	if code != "invalid" {
		t.Errorf("refusal code = %q, want invalid", code)
	}
	if n := p.stagedDeliveries(t); n != 0 {
		t.Errorf("%d deliveries staged behind a refused claim, want 0", n)
	}
}
