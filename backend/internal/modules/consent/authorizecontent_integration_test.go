// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The message that goes is the message that was authorized.
//
// Both phases stamp a fingerprint of the wording, and until now nothing read
// the staging one back — so a body edited between staging and transmit was
// authorized as one message and sent as another, with the record showing two
// different fingerprints and nobody comparing them.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	stagedSubject = "Your April invoice"
	stagedBody    = "The invoice for April is attached, due on the 30th."
	// The distinguishing fragment of the wording refusal, so a test can tell it
	// from a refusal about the recipient.
	wordingRefusal = "edited after it was authorized"
)

// stageThenTransmit runs BOTH real writers over one delivery: the staging
// decision through AuthorizeStagingTx, then the transmit decision through
// AuthorizeTransmit with whatever wording the caller says goes.
//
// Through the real writers on purpose. A hand-planted staging row is where this
// defect hides — the fixtures elsewhere in this package omit
// content_fingerprint entirely, so a test built on one would compare against a
// NULL and pass whatever the code did.
func stageThenTransmit(
	t *testing.T, e *resolveEnv, delivery ids.UUID, sentSubject, sentBody string,
) commsauthz.TransmitTicket {
	t.Helper()
	// A live confirm link, which is what authorizes a record-confirmation send.
	// The category is chosen for how LITTLE it needs: the subject of these tests
	// is the fingerprint, and an invoice or a deal would put a chain of
	// company, employment and document rows between the test and its point.
	//
	// Through the sibling's own fixture helper, which already argues why direct
	// SQL is right here: the real writer mints a token AND stages the mail that
	// carries it, so reaching for it would need a stager, a vault and a runner.
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	recipients := []connector.Recipient{{Email: e.address}}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients,
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
		})
		return err
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery,
		Attempt:    1,
		Recipients: recipients,
		Subject:    sentSubject,
		Body:       sentBody,
	})
	if err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
	return ticket
}

// TestTheSameWordingStillTransmits is the positive control, and it comes first
// because without it every refusal below could be a refusal about something
// else entirely — a category, a suppression, a missing basis.
func TestTheSameWordingStillTransmits(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	ticket := stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)
	if !ticket.Allowed {
		t.Fatalf("an unedited message was refused: %q — the comparison is failing a send "+
			"whose wording never changed", ticket.Reason)
	}
}

// TestABodyEditedAfterStagingDoesNotTransmit is the defect this closes.
func TestABodyEditedAfterStagingDoesNotTransmit(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	ticket := stageThenTransmit(t, e, delivery, stagedSubject,
		"Actually, pay this to a different account entirely.")
	if ticket.Allowed {
		t.Fatal("a message edited after it was authorized still transmitted: the decision " +
			"on record describes wording that is not what would go")
	}
	// The WORDING reason specifically. Asserting only that it refused would pass
	// against a refusal about the recipient, which is how the first version of
	// these tests proved nothing.
	if !strings.Contains(ticket.Reason, wordingRefusal) {
		t.Errorf("the refusal reads %q, not the wording reason: this test would pass "+
			"against a refusal about something else entirely", ticket.Reason)
	}
}

// TestASubjectEditedAfterStagingDoesNotTransmit holds the other half of the
// digest. The fingerprint is over subject AND body, separated by a NUL so a
// sentence moved between them is a different message — a subject rewritten
// while the body stands is exactly the edit a body-only check would miss.
func TestASubjectEditedAfterStagingDoesNotTransmit(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	ticket := stageThenTransmit(t, e, delivery, "URGENT: your account is suspended", stagedBody)
	if ticket.Allowed {
		t.Fatal("a message whose subject was rewritten after authorization still transmitted")
	}
	if !strings.Contains(ticket.Reason, wordingRefusal) {
		t.Errorf("the refusal reads %q, not the wording reason", ticket.Reason)
	}
}

// TestADeliveryWithNoStagedWordingIsNotTreatedAsEdited holds the absence case.
//
// A delivery staged before this column carried anything, or one whose staging
// rows erasure scrubbed, has no fingerprint to compare. Inventing a mismatch
// from an absence would park every legacy delivery in flight when this shipped.
//
// Asserted at the comparison rather than through AuthorizeTransmit, and that
// distinction is the point: a delivery with NO staging row is also refused for
// having no staged claim to resolve a category from, so a ticket-level
// assertion would pass on that refusal and prove nothing about the wording. The
// question here is narrow — does an absent fingerprint read as "changed"? — so
// it is asked of the function that answers it.
func TestADeliveryWithNoStagedWordingIsNotTreatedAsEdited(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	var recorded bool
	var staged []byte
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		staged, recorded, err = stagedWordingFor(e.ctx, tx, delivery)
		return err
	}); err != nil {
		t.Fatalf("reading the staged wording: %v", err)
	}
	if recorded {
		t.Fatalf("a delivery that was never staged reports a fingerprint of %d bytes", len(staged))
	}
	// And the CALLER's reading of that, which is the branch that decides:
	// wordingDiffersFromStaging must answer false on an absence rather than
	// treating "no record" as "changed". Asked through the caller and not
	// through the comparison alone, because the comparison never sees the
	// absent case — the early return above is what handles it, and a test that
	// skipped past it would leave that return unheld.
	var differs bool
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		differs, err = e.gate.wordingDiffersFromStaging(e.ctx, tx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: 1,
			Subject: stagedSubject, Body: stagedBody,
		})
		return err
	}); err != nil {
		t.Fatalf("asking whether the wording differs: %v", err)
	}
	if differs {
		t.Error("an absent staging fingerprint reads as an edited message, which would park " +
			"every delivery staged before this column carried anything")
	}
}

// TestTheTransmitRowRecordsTheWordingThatWouldHaveGone holds what the record
// says after a refusal.
//
// The transmit row stamps the wording it was ASKED about, not the staged one:
// an auditor asking "what did somebody try to send" is owed the message that
// was actually presented, and the staging row beside it holds the other.
func TestTheTransmitRowRecordsTheWordingThatWouldHaveGone(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	edited := "Actually, pay this to a different account entirely."

	stageThenTransmit(t, e, delivery, stagedSubject, edited)

	var fingerprints [][]byte
	rows, err := e.owner.Query(context.Background(), `
		SELECT content_fingerprint FROM communication_decision
		 WHERE delivery_id = $1 ORDER BY phase`, delivery)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sum []byte
		if err := rows.Scan(&sum); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fingerprints = append(fingerprints, sum)
	}
	rows.Close()
	if len(fingerprints) != 2 {
		t.Fatalf("the delivery carries %d decisions, want a staging and a transmit row",
			len(fingerprints))
	}
	// SendingDigest, which is what both writers stamp: the row records what a
	// recipient can read, markup included, and this asserts the stored bytes.
	staged := SendingDigest(stagedSubject, stagedBody, "")
	sent := SendingDigest(stagedSubject, edited, "")
	// Ordered by phase: 'staging' sorts before 'transmit'.
	if string(fingerprints[0]) != string(staged[:]) {
		t.Error("the staging row does not record the wording that was authorized")
	}
	if string(fingerprints[1]) != string(sent[:]) {
		t.Error("the transmit row does not record the wording that was presented")
	}
}

// TestARefusedSendKeepsItsOwnReason holds the ordering.
//
// The wording check runs LAST and only on a send the engine would otherwise
// allow. A delivery already refused about its RECIPIENT — a suppression, a
// missing basis — must keep that reason: replacing it with "the wording
// changed" would tell an operator to re-approve a message somebody had
// objected to, which is the one reading that turns a refusal into an invitation.
func TestARefusedSendKeepsItsOwnReason(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)

	// Staged with a live link, so the staging phase records a fingerprint...
	recipients := []connector.Recipient{{Email: e.address}}
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients,
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
		})
		return err
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	// ...then the link is spent, so the recipient no longer authorizes anything.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE confirm_token SET consumed_at = now() WHERE contact_id = $1`, e.contact); err != nil {
		t.Fatalf("spending the link: %v", err)
	}

	// And the body is edited too, so BOTH refusals are available and the test
	// can tell which one the ticket reports.
	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: recipients,
		Subject: stagedSubject, Body: "Actually, pay this to a different account entirely.",
	})
	if err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
	if ticket.Allowed {
		t.Fatal("a send whose recipient no longer authorizes it still transmitted")
	}
	if strings.Contains(ticket.Reason, "edited after it was authorized") {
		t.Errorf("the refusal reads %q: a recipient-level refusal was overwritten by the "+
			"wording check, so an operator is told to re-approve rather than that the "+
			"recipient refused", ticket.Reason)
	}
}

// TestARetryOfAnUneditedMessageStillTransmits holds the retry ladder.
//
// A delivery is staged ONCE and transmitted as many times as the dispatcher
// retries, each attempt taking a fresh decision. The comparison is against the
// staging row, which does not move — so an unedited message must clear it on
// every attempt. A check that parked on the second try would turn every
// transient provider failure into a permanent one.
func TestARetryOfAnUneditedMessageStillTransmits(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	recipients := []connector.Recipient{{Email: e.address}}
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients,
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
		})
		return err
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}

	// Three attempts of the same message, as a retrying dispatcher makes them.
	for attempt := 1; attempt <= 3; attempt++ {
		ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients,
			Subject: stagedSubject, Body: stagedBody,
		})
		if err != nil {
			t.Fatalf("attempt %d: authorizing the transmit: %v", attempt, err)
		}
		if !ticket.Allowed {
			t.Fatalf("attempt %d of an unedited message was refused: %q — a retry must not "+
				"read as an edit, or a transient failure becomes a permanent one",
				attempt, ticket.Reason)
		}
	}
}

// TestMarkupEditedAfterStagingDoesNotTransmit holds the half of the message
// most recipients actually read.
//
// A mail carries two bodies. A client that renders markup shows HTMLBody and
// never Body, so a fingerprint over the plain text alone would let the visible
// half be rewritten between the decision and the send while the record went on
// claiming the message was unchanged. The plain body here is IDENTICAL in both
// phases, so nothing but the markup can account for the refusal.
func TestMarkupEditedAfterStagingDoesNotTransmit(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	recipients := []connector.Recipient{{Email: e.address}}

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients,
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
			HTMLBody:   "<p>The invoice for April is attached, due on the 30th.</p>",
		})
		return err
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}

	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: recipients,
		Subject: stagedSubject, Body: stagedBody,
		HTMLBody: "<p>Pay this to a different account entirely.</p>",
	})
	if err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
	if ticket.Allowed {
		t.Fatal("a message whose MARKUP was rewritten after authorization still transmitted: " +
			"the half a rendering client shows is outside what was checked")
	}
	if !strings.Contains(ticket.Reason, wordingRefusal) {
		t.Errorf("the refusal reads %q, not the wording reason", ticket.Reason)
	}
}

// TestADeliveryStagedUnderTheOlderDigestStillTransmits holds the deploy.
//
// A delivery staged before this change carries the two-field digest, which
// covered subject and body and knew nothing about markup. Its wording has not
// changed — the shape of the question has — and refusing it would park every
// message in flight at the moment this shipped.
//
// The staging row is written by hand HERE, and that is the point rather than a
// shortcut: the current writer cannot produce the old shape, so the only way to
// test the compatibility path is to plant what the old writer left behind.
func TestADeliveryStagedUnderTheOlderDigestStillTransmits(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)

	legacy := WordingDigest(stagedSubject, stagedBody)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   requested_category, resolved_category, verdict, reason_code,
		   content_fingerprint, mode, actor)
		VALUES ($1, 0, $2, $3, 'staging', 'record_confirmation', 'record_confirmation',
		        'allow', 'confirmation_link_live', $4, 'enforce', 'test')`,
		delivery, ids.NewV7(), e.address, legacy[:]); err != nil {
		t.Fatalf("planting the pre-deploy staging row: %v", err)
	}

	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1,
		Recipients: []connector.Recipient{{Email: e.address}},
		Subject:    stagedSubject, Body: stagedBody,
	})
	if err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
	if !ticket.Allowed {
		t.Fatalf("a delivery staged under the older digest was refused: %q — every message "+
			"in flight at the deploy would park", ticket.Reason)
	}
}

// TestTheOlderDigestIsNotAcceptedForAMessageWithMarkup is the other half.
//
// A message that HAS markup cannot have been staged under a digest that never
// saw markup, so accepting the older shape there would take a fingerprint that
// answers a narrower question than the one being asked — and let the markup be
// anything at all.
func TestTheOlderDigestIsNotAcceptedForAMessageWithMarkup(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)

	legacy := WordingDigest(stagedSubject, stagedBody)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   requested_category, resolved_category, verdict, reason_code,
		   content_fingerprint, mode, actor)
		VALUES ($1, 0, $2, $3, 'staging', 'record_confirmation', 'record_confirmation',
		        'allow', 'confirmation_link_live', $4, 'enforce', 'test')`,
		delivery, ids.NewV7(), e.address, legacy[:]); err != nil {
		t.Fatalf("planting the pre-deploy staging row: %v", err)
	}

	ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1,
		Recipients: []connector.Recipient{{Email: e.address}},
		Subject:    stagedSubject, Body: stagedBody,
		HTMLBody: "<p>Pay this to a different account entirely.</p>",
	})
	if err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
	if ticket.Allowed {
		t.Fatal("markup rode a fingerprint that never covered markup: the compatibility " +
			"path must not admit a message the older digest could not have described")
	}
}
