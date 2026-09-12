// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The counter counts transmit decisions the transaction COMMITTED, once each.
//
// Two things can go wrong and neither is visible from the counter itself. The
// insert takes ON CONFLICT DO NOTHING on
// (decision_set_id, recipient_address, phase), so one address reached twice in
// a single set is one row — counting per loop iteration would report a Cc to
// the same contact as a second decision. And AuthorizeTransmit can fail after
// the rows are written, rolling them back, so counting inside the transaction
// would report decisions no row holds.
//
// Through the REAL writer, on a real database. A test that called
// countDecision itself would prove the counter adds up and nothing about
// whether the send path reaches it.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func countedNow(t *testing.T, key DecisionCount) uint64 {
	t.Helper()
	return DecisionTotals()[key]
}

// allowedRecordConfirmation is the label combination the fixture below produces:
// the engine allows a record confirmation, under the shipped enforcing posture.
var allowedRecordConfirmation = DecisionCount{
	Verdict:  commsauthz.VerdictAllow,
	Category: commsauthz.CategoryRecordConfirmation,
	Mode:     commsauthz.ModeEnforce,
}

func TestATransmitDecisionIsCountedOnce(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	before := countedNow(t, allowedRecordConfirmation)

	ticket := stageThenTransmit(t, e, delivery, stagedSubject, stagedBody)
	if !ticket.Allowed {
		t.Fatalf("the fixture message was refused: %q — this test needs an ALLOWED transmit, "+
			"so a refusal here means it is counting the wrong verdict", ticket.Reason)
	}

	if got := countedNow(t, allowedRecordConfirmation) - before; got != 1 {
		t.Fatalf("one transmitted recipient moved the counter by %d, want 1 — the transmit "+
			"writer does not reach countDecision, so /metrics is blind to the last judgement "+
			"taken before mail leaves", got)
	}
}

// One address reached twice in a single decision set is one row, and must be
// one count. This is the case the ON CONFLICT exists for.
func TestARecipientNamedTwiceIsCountedOnce(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	before := countedNow(t, allowedRecordConfirmation)

	// The same address twice, which is what a To and a Cc to one contact is.
	transmitTo(t, e, delivery, []connector.Recipient{{Email: e.address}, {Email: e.address}})

	if got := countedNow(t, allowedRecordConfirmation) - before; got != 1 {
		t.Fatalf("one address named twice moved the counter by %d, want 1 — the count is taken "+
			"per recipient rather than per inserted row, so a Cc to the same contact reads as "+
			"a second decision", got)
	}
}

// The defect both reviewers found: a transaction that rolls back after the
// decisions are written must leave NO count behind.
//
// The rollback is induced the way production induces it — the caller's own
// transaction fails once AuthorizeTransmit has committed nothing yet. Without
// the after-commit counting this test reports one, and the refusal rate an
// operator alerts on climbs on rows that do not exist.
func TestARolledBackTransmitCountsNothing(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	before := countedNow(t, allowedRecordConfirmation)

	// A transmit whose transaction the legacy-gate arm aborts. The wording
	// comparison is the reachable spelling of the same shape: it runs after
	// recordDecisions and its error path rolls the rows back.
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: []connector.Recipient{{Email: e.address}},
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
		}); err != nil {
			return err
		}
		return errors.New("the caller refuses after the decisions are written")
	}); err == nil {
		t.Fatal("the probe transaction was supposed to fail")
	}

	var rows int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM communication_decision WHERE delivery_id = $1`, delivery).Scan(&rows); err != nil {
		t.Fatalf("counting the rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("the rolled-back transaction left %d rows — this test needs the rollback to "+
			"have happened for its count assertion to mean anything", rows)
	}
	if got := countedNow(t, allowedRecordConfirmation) - before; got != 0 {
		t.Fatalf("a rolled-back decision moved the counter by %d, want 0 — the count is taken "+
			"inside the transaction, so /metrics reports decisions communication_decision "+
			"does not hold", got)
	}
}

// transmitTo runs the real transmit writer over one delivery to the given
// recipients, after staging the same message.
func transmitTo(t *testing.T, e *resolveEnv, delivery ids.UUID, to []connector.Recipient) {
	t.Helper()
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(14*24*time.Hour), nil)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: to,
			Context:    commsauthz.CategoryRecordConfirmation,
			Subject:    stagedSubject,
			Body:       stagedBody,
		})
		return err
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	if _, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery,
		Attempt:    1,
		Recipients: to,
		Subject:    stagedSubject,
		Body:       stagedBody,
	}); err != nil {
		t.Fatalf("authorizing the transmit: %v", err)
	}
}
