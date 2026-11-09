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

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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

	l.recordReceiptSent(t)
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

// recordReceiptSent closes the lane's receipt through the send transition the
// dispatcher uses.
func (l *confirmLaneEnv) recordReceiptSent(t *testing.T) {
	t.Helper()
	var delivery ids.UUID
	if err := l.e.Pool.QueryRow(context.Background(),
		`SELECT id FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).Scan(&delivery); err != nil {
		t.Fatal(err)
	}
	store := comms.NewStore(InstallationDB(l.e.Pool), func() time.Time { return clocktest.Now(t) }, activities.NewStore(InstallationDB(l.e.Pool)))
	if err := store.RecordSent(sendWorkerScope(l.ctx), delivery, connector.SendReceipt{ProviderMessageID: "relay-accepted"}); err != nil {
		t.Fatalf("recording the receipt sent: %v", err)
	}
}

// liveNoticeTargets counts the missed-receipt notices still naming target.
func (l *confirmLaneEnv) liveNoticeTargets(t *testing.T, target ids.UUID) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed' AND target_id = $1`, target)
}

// A subject erased before the deadline is never raised, and a notice already
// raised about them is taken back and stops naming them.
func TestAnErasedSubjectKeepsNoMissedReceiptNotice(t *testing.T) {
	t.Run("erased before the deadline", func(t *testing.T) {
		l := receiptLane(t, receiptOwed)
		l.object(t)
		var dueAt time.Time
		if err := l.e.Pool.QueryRow(context.Background(),
			`SELECT delivery_due_at FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).Scan(&dueAt); err != nil {
			t.Fatal(err)
		}
		if err := ComposedEraser(l.e.Pool, nil, nil).EraseContact(l.e.Admin(), l.contact, "Art. 17 request"); err != nil {
			t.Fatalf("erasing the subject: %v", err)
		}
		if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, keyvault.NewMemory(), dueAt.Add(time.Minute)); err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		if n := l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed'`); n != 0 {
			t.Fatalf("%d notices raised about a subject erased before the deadline", n)
		}
	})
	t.Run("erased after the notice", func(t *testing.T) {
		l := receiptLane(t, receiptOwed)
		l.e.WsExec(t, `DELETE FROM contact_email WHERE contact_id = $1`, l.contact)
		l.object(t)
		if l.liveNoticeTargets(t, l.contact) == 0 {
			t.Fatal("no notice to take back; the test proves nothing")
		}
		if err := ComposedEraser(l.e.Pool, nil, nil).EraseContact(l.e.Admin(), l.contact, "Art. 17 request"); err != nil {
			t.Fatalf("erasing the subject: %v", err)
		}
		if n := l.liveNoticeTargets(t, l.contact); n != 0 {
			t.Fatalf("%d notices still name the erased subject", n)
		}
		if n := l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed' AND retracted_at IS NULL`); n != 0 {
			t.Fatalf("%d notices about the erased subject are still open", n)
		}
	})
}

// A backlog longer than one page is raised and settled across passes.
func TestAReceiptBacklogLongerThanAPageIsRaisedAndSettledInFull(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.object(t)
	second := l.e.SeedContact(t, "Second Refusal", &l.e.Rep1)
	l.e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'second@example.test', true, 'manual', 'human:test')`, second)
	l.e.WsExec(t, `UPDATE contact SET address_country = upper($2) WHERE id = $1`, second, string(receiptOwed))
	if err := l.store.Suppress(l.ctx, consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](second), Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
	}); err != nil {
		t.Fatal(err)
	}
	var dueAt time.Time
	if err := l.e.Pool.QueryRow(context.Background(),
		`SELECT max(delivery_due_at) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).Scan(&dueAt); err != nil {
		t.Fatal(err)
	}
	onePerPage := comms.WithReceiptPage(1)
	sweep := func() {
		t.Helper()
		if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, keyvault.NewMemory(), dueAt.Add(time.Minute), onePerPage); err != nil {
			t.Fatalf("sweeping: %v", err)
		}
	}
	open := func() int {
		return l.e.WsCount(t, `SELECT count(DISTINCT dedupe_key) FROM notice WHERE kind = 'optout_receipt_missed' AND retracted_at IS NULL`)
	}
	sweep()
	sweep()
	if n := open(); n != 2 {
		t.Fatalf("%d receipts raised after two one-row passes, want both", n)
	}
	store := comms.NewStore(InstallationDB(l.e.Pool), func() time.Time { return clocktest.Now(t) }, activities.NewStore(InstallationDB(l.e.Pool)))
	rows, err := l.e.Pool.Query(context.Background(), `SELECT id FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatal(err)
	}
	for _, delivery := range deliveries {
		if err := store.RecordSent(sendWorkerScope(l.ctx), delivery, connector.SendReceipt{ProviderMessageID: "relay-accepted"}); err != nil {
			t.Fatal(err)
		}
	}
	sweep()
	sweep()
	if n := open(); n != 0 {
		t.Fatalf("%d notices still open after their receipts were sent and two one-row passes ran", n)
	}
}

// A receipt owed where no mail lane can send it is raised, not dropped.
func TestAReceiptOwedWithNoMailLaneReachesThePrivacyOperators(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.store = consent.NewStore(InstallationDB(l.e.Pool)).WithMissedReceipts(receiptAlarm{db: InstallationDB(l.e.Pool)})
	l.object(t)

	bodies := l.missedReceiptNotices(t)
	if len(bodies) == 0 || !strings.Contains(bodies[0], "no mail lane") {
		t.Fatalf("a receipt owed with no lane raised %q", bodies)
	}
}

// A lead's press asks the lead's rules and raises the receipt it cannot send.
func TestALeadsPressThatOwesAReceiptReachesThePrivacyOperators(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	company := l.e.SeedCompany(t, "Receipt Company", nil)
	l.e.WsExec(t, `UPDATE company SET address_country = upper($2) WHERE id = $1`, company, string(receiptOwed))
	lead, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{
		FullName: new("Lead Refusal"), Email: new("lead@example.test"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	l.e.WsExec(t, `UPDATE lead SET company_id = $2 WHERE id = $1`, lead.Id, company)
	if err := InstallationDB(l.e.Pool).Tx(l.ctx, func(tx pgx.Tx) error {
		stopped, err := l.store.StopForCredentialTx(l.ctx, tx, consent.WithdrawalRef{
			LeadID: ids.From[ids.LeadKind](ids.UUID(lead.Id)), Address: "lead@example.test",
			Scope: consent.WithdrawalScopeAllMarketing,
		})
		if err == nil && !stopped {
			t.Error("the press recorded no stop")
		}
		return err
	}); err != nil {
		t.Fatalf("pressing the lead's link: %v", err)
	}
	var body string
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT body FROM notice WHERE kind = 'optout_receipt_missed' AND target_type = 'lead' AND target_id = $1 LIMIT 1`,
		ids.UUID(lead.Id)).Scan(&body); err != nil {
		t.Fatalf("no notice for the lead's owed receipt: %v", err)
	}
	if !strings.Contains(body, "lead") {
		t.Errorf("the notice does not say a lead's receipt is not sent: %q", body)
	}
}

// Two different refusals on one day each owe their own receipt.
func TestTwoDifferentSameDayWithdrawalsEachGetAReceipt(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	for _, key := range []string{"receipt_one", "receipt_two"} {
		l.e.WsExec(t, `INSERT INTO consent_purpose (key, label, class) VALUES ($1, $1, 'marketing')`, key)
	}
	for _, key := range []string{"receipt_one", "receipt_two"} {
		withdrawn, err := l.store.PublicWithdrawAll(l.ctx, ids.From[ids.ContactKind](l.contact), []string{key})
		if err != nil || len(withdrawn) != 1 {
			t.Fatalf("withdrawing %s: %v %v", key, withdrawn, err)
		}
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`); n != 2 {
		t.Fatalf("%d receipts for two refusals, want 2", n)
	}
	if bodies := l.missedReceiptNotices(t); len(bodies) != 0 {
		t.Fatalf("the second refusal's receipt was reported missed: %q", bodies)
	}
}

// A press that withdraws the same purpose again after a re-grant is a second
// refusal and owes a second receipt.
func TestARepeatedSameDayWithdrawalOfOnePurposeGetsItsOwnReceipt(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.e.WsExec(t, `INSERT INTO consent_purpose (key, label, class) VALUES ('receipt_again', 'receipt_again', 'marketing')`)
	var purpose ids.PurposeID
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT id FROM consent_purpose WHERE key = 'receipt_again'`).Scan(&purpose); err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](l.contact)
	for press := range 2 {
		if press > 0 {
			if _, err := l.store.Record(l.ctx, consent.RecordInput{
				ContactID: contact, PurposeID: purpose, NewState: "granted", PolicyText: new("Monthly offers."),
			}); err != nil {
				t.Fatalf("granting again: %v", err)
			}
		}
		withdrawn, err := l.store.PublicWithdrawAll(l.ctx, contact, []string{"receipt_again"})
		if err != nil || len(withdrawn) != 1 {
			t.Fatalf("press %d: %v %v", press+1, withdrawn, err)
		}
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`); n != 2 {
		t.Fatalf("%d receipts for two refusals of one purpose, want 2", n)
	}
	if bodies := l.missedReceiptNotices(t); len(bodies) != 0 {
		t.Fatalf("the second refusal's receipt was reported missed: %q", bodies)
	}
}

// A receipt the sweep read as overdue and an erasure scrubbed before the raise
// raises nothing about the erased subject.
func TestAReceiptErasedBetweenTheReadAndTheRaiseIsNotRaised(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	l.object(t)
	var dueAt time.Time
	if err := l.e.Pool.QueryRow(context.Background(),
		`SELECT delivery_due_at FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).Scan(&dueAt); err != nil {
		t.Fatal(err)
	}
	ctx, err := installationJobCtx(context.Background(), identity.NewService(l.e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	ctx = sendWorkerScope(ctx)
	db := InstallationDB(l.e.Pool)
	store := comms.NewStore(db, func() time.Time { return clocktest.Now(t) }, activities.NewStore(db))
	now := dueAt.Add(time.Minute)
	overdue, err := store.UnraisedOverdueReceipts(ctx, now, noticekeys.OptOutReceiptPrefix)
	if err != nil || len(overdue) != 1 {
		t.Fatalf("read %d overdue receipts (%v), want the one", len(overdue), err)
	}
	if err := ComposedEraser(l.e.Pool, nil, nil).EraseContact(l.e.Admin(), l.contact, "Art. 17 request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	if failed := (receiptAlarm{db: db}).raiseOverdue(ctx, store, now, overdue); failed != 0 {
		t.Fatalf("%d raises failed", failed)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed'`); n != 0 {
		t.Fatalf("%d notices raised about a subject erased after the read", n)
	}
}
