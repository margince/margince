// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A required opt-out receipt that does not go out reaches the privacy
// operators as a notice, and the refusal it is about always stands.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// receiptOwed is a code no shipped pack holds, owing a 24-hour receipt.
const receiptOwed jurisdiction.Code = "qs"

var registerReceiptRules sync.Once

func receiptLane(t *testing.T, code jurisdiction.Code) *confirmLaneEnv {
	t.Helper()
	registerReceiptRules.Do(func() {
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: receiptOwed, Version: 1, OptOutAcknowledgement: true, OptOutAcknowledgementWithin: 24 * time.Hour,
		})
	})
	l := setupConfirmLane(t)
	// The installation's administrator is the privacy operator who is told.
	l.e.WsExec(t, `INSERT INTO role_assignment (user_id, role_id) SELECT $1, id FROM role WHERE key = 'admin'`, l.e.AdminUser)
	l.store = l.store.WithMissedReceipts(receiptAlarm{db: InstallationDB(l.e.Pool)})
	if code != "" {
		l.livesIn(t, code)
	}
	return l
}

func (l *confirmLaneEnv) object(t *testing.T) {
	t.Helper()
	if err := l.store.Suppress(l.ctx, consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](l.contact), Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
	}); err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, l.contact); n != 1 {
		t.Fatalf("%d suppressions, want the refusal recorded whatever happened to its receipt", n)
	}
}

// missedReceiptNotices answers the live missed-receipt notices about the lane's
// contact, and their bodies.
func (l *confirmLaneEnv) missedReceiptNotices(t *testing.T) []string {
	t.Helper()
	rows, err := l.e.Pool.Query(context.Background(), `
		SELECT body FROM notice
		 WHERE kind = 'optout_receipt_missed' AND target_type = 'contact' AND target_id = $1
		   AND retracted_at IS NULL AND starts_with(dedupe_key, 'optout-receipt:optout-ack-')`, l.contact)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var bodies []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func TestAReceiptThatCannotBePreparedReachesThePrivacyOperators(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	// The receipt's language cannot be read, the way the preparation test in
	// controllermail_integration_test.go breaks it.
	l.store = l.store.WithMailLanguage(consent.MailLanguageFunc(func(ctx context.Context, tx pgx.Tx) (string, error) {
		_, err := tx.Exec(ctx, `SELECT 1/0`)
		return "", err
	}))
	l.object(t)

	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`); n != 0 {
		t.Fatalf("%d receipts staged, want none: its preparation failed", n)
	}
	bodies := l.missedReceiptNotices(t)
	if len(bodies) == 0 {
		t.Fatal("the receipt could not be prepared and no privacy operator was told")
	}
	if !strings.Contains(bodies[0], "preparing it failed") {
		t.Errorf("the notice does not say why: %q", bodies[0])
	}
}

func TestAReceiptOwedToASubjectWithNoAddressReachesThePrivacyOperators(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.e.WsExec(t, `DELETE FROM contact_email WHERE contact_id = $1`, l.contact)
	l.object(t)

	bodies := l.missedReceiptNotices(t)
	if len(bodies) == 0 || !strings.Contains(bodies[0], "no email address") {
		t.Fatalf("a required receipt with nowhere to go raised %q", bodies)
	}
}

func TestARefusalThatOwesNoReceiptRaisesNothing(t *testing.T) {
	l := receiptLane(t, "")
	l.e.WsExec(t, `DELETE FROM contact_email WHERE contact_id = $1`, l.contact)
	l.object(t)

	if bodies := l.missedReceiptNotices(t); len(bodies) != 0 {
		t.Fatalf("no rule owes this receipt and the operators were told anyway: %q", bodies)
	}
}

// An overdue receipt is raised once per stop act, and taken back when it is sent.
func TestAnOverdueReceiptIsRaisedOnceAndSettledWhenSent(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.object(t)
	var dueAt time.Time
	if err := l.e.Pool.QueryRow(context.Background(),
		`SELECT delivery_due_at FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).Scan(&dueAt); err != nil {
		t.Fatalf("reading the receipt's deadline: %v", err)
	}
	vault := keyvault.NewMemory()
	for range 2 {
		if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, vault, dueAt.Add(time.Minute)); err != nil {
			t.Fatalf("sweeping: %v", err)
		}
	}
	bodies := l.missedReceiptNotices(t)
	operators := l.e.WsCount(t, `SELECT count(DISTINCT recipient_user_id) FROM notice WHERE kind = 'optout_receipt_missed'`)
	if len(bodies) == 0 || len(bodies) != operators {
		t.Fatalf("%d notices for %d operators after two sweeps, want one each", len(bodies), operators)
	}
	if !strings.Contains(bodies[0], "has not been delivered") {
		t.Errorf("the notice does not say the receipt is overdue: %q", bodies[0])
	}

	l.e.WsExec(t, `UPDATE comms_outbound SET status = 'sent', sent_at = now() WHERE template_key = 'optout_acknowledgement'`)
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, vault, dueAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if bodies := l.missedReceiptNotices(t); len(bodies) != 0 {
		t.Fatalf("the receipt was sent and %d notices still stand", len(bodies))
	}
}

// brokenAlarm fails the way a database fault inside the alarm would.
type brokenAlarm struct{}

func (brokenAlarm) RaiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, _ consent.MissedReceipt) error {
	_, err := tx.Exec(ctx, `SELECT 1/0`)
	return err
}

// An alarm that fails is logged; it never takes the refusal down with it.
func TestAFailingAlarmCannotRollBackTheRefusal(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.e.WsExec(t, `DELETE FROM contact_email WHERE contact_id = $1`, l.contact)
	l.store = l.store.WithMissedReceipts(brokenAlarm{})
	l.object(t)
}
