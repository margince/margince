// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the receipt Decree 91/2020/ND-CP Art. 20 owes says, and when it is
// queued: with the refusal, printing when the refusal arrived and when sending
// stopped.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// owingVietnameseReceipt binds the store to a pack that owes the receipt on the
// Vietnamese pack's terms, with stager as the lane.
func owingVietnameseReceipt(t *testing.T, e *channelConsentEnv, stager ConfirmationSender) {
	t.Helper()
	code := testJurisdiction(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: code, Version: 1, OptOutAcknowledgement: true,
		OptOutAcknowledgementWithin: time.Hour, OptOutAcknowledgementUTCOffset: 7 * time.Hour,
	})
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET address_country = upper($2) WHERE id = $1`, e.contact, string(code)); err != nil {
		t.Fatal(err)
	}
	e.store = e.store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/")
}

// TestAReceiptPrintsWhenTheRefusalArrivedAndWhenSendingStopped records a call
// taken two hours before it was typed in.
func TestAReceiptPrintsWhenTheRefusalArrivedAndWhenSendingStopped(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := &recordingStager{}
	owingVietnameseReceipt(t, e, stager)
	called := clocktest.Now(t).Add(-2 * time.Hour).Truncate(time.Minute)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, ReceivedAt: &called,
	}); err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	var stopped time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT recorded_at FROM communication_suppression WHERE contact_id = $1`, e.contact).Scan(&stopped); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	vietnam := time.FixedZone("UTC+07:00", 7*60*60)
	body := stager.seen.Rendered.Body
	for name, at := range map[string]time.Time{"received": called, "stopped": stopped} {
		if want := at.In(vietnam).Format(refusalInstantLayout); !strings.Contains(body, want) {
			t.Errorf("the receipt does not print the %s time as %s:\n%s", name, want, body)
		}
	}
	if strings.Contains(body, called.UTC().Format(refusalInstantLayout)) {
		t.Errorf("the receipt prints the received time in UTC:\n%s", body)
	}
	mail := stager.seen.Rendered.Subject + body + RenderControllerPresentation(stager.seen.Rendered, "Example Company", "")
	if strings.Contains(mail, "Margince") {
		t.Errorf("the receipt names Margince:\n%s", mail)
	}
}

// TestARefusalCannotArriveAfterItIsRecorded. A time later than the record is a
// typing mistake, and the receipt would print it as fact.
func TestARefusalCannotArriveAfterItIsRecorded(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	owingVietnameseReceipt(t, e, &recordingStager{})
	later := clocktest.Now(t).Add(time.Hour)

	err := e.store.Suppress(e.ctx, SuppressInput{ContactID: e.contact, Kind: commsauthz.ReasonObjection, ReceivedAt: &later})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Field != fieldReceivedAt {
		t.Fatalf("a refusal received in the future answered %v, want a validation error on received_at", err)
	}
	var stops int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, e.contact).Scan(&stops); err != nil {
		t.Fatal(err)
	}
	if stops != 0 {
		t.Errorf("%d stops recorded for a refused input, want 0", stops)
	}
}

// probingStager looks at the refusal from inside the staging call and from a
// second connection.
type probingStager struct {
	recordingStager
	outside *pgx.Conn
	// insideSees and outsideSees count the contact's stops each side saw.
	insideSees, outsideSees int
}

func (p *probingStager) QueueConfirmationTx(ctx context.Context, tx pgx.Tx, in ConfirmationSend) (ids.UUID, error) {
	const stops = `SELECT count(*) FROM communication_suppression WHERE contact_id = $1`
	if err := tx.QueryRow(ctx, stops, in.ContactID).Scan(&p.insideSees); err != nil {
		return ids.UUID{}, err
	}
	if err := p.outside.QueryRow(ctx, stops, in.ContactID).Scan(&p.outsideSees); err != nil {
		return ids.UUID{}, err
	}
	return p.recordingStager.QueueConfirmationTx(ctx, tx, in)
}

// TestTheReceiptIsQueuedInsideTheRefusalsTransaction. The stager sees the stop
// its receipt answers while no other connection can. So the two commit
// together, and the receipt goes out the moment they do (Art. 20.3).
func TestTheReceiptIsQueuedInsideTheRefusalsTransaction(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := &probingStager{outside: e.owner}
	owingVietnameseReceipt(t, e, stager)

	if err := e.store.Suppress(e.ctx, SuppressInput{ContactID: e.contact, Kind: commsauthz.ReasonObjection}); err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	if stager.calls != 1 {
		t.Fatalf("%d receipts staged for one refusal, want 1", stager.calls)
	}
	if stager.insideSees != 1 || stager.outsideSees != 0 {
		t.Errorf("staging saw %d stops and a second connection %d, want 1 and 0: the receipt "+
			"was not queued in the refusal's own transaction", stager.insideSees, stager.outsideSees)
	}
	// One refusal, one message id: the ledger's unique index refuses a second
	// receipt for the same stop.
	var stopID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM communication_suppression WHERE contact_id = $1`, e.contact).Scan(&stopID); err != nil {
		t.Fatal(err)
	}
	if want := acknowledgementMessageID(stopID); stager.seen.MessageID != want {
		t.Errorf("the receipt's message id is %q, want %q, derived from the stop it answers", stager.seen.MessageID, want)
	}
}
