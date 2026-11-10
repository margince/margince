// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A missed receipt whose alarm cannot be raised is kept as owed until a notice
// raises it, and keeping it never undoes the refusal.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// failingAlarm fails to raise: with a plain error, or with a statement the
// database refuses inside the alarm's savepoint.
type failingAlarm struct{ sql string }

func (a failingAlarm) RaiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, _ MissedReceipt) error {
	if a.sql == "" {
		return errors.New("nobody can be told")
	}
	_, err := tx.Exec(ctx, a.sql)
	return err
}

type raisedAlarm struct{}

func (raisedAlarm) RaiseMissedReceiptTx(context.Context, pgx.Tx, MissedReceipt) error { return nil }

// owedReceipts reads the receipts kept as owed about the lane's contact,
// through the sweep's own reader.
func owedReceipts(t *testing.T, e *channelConsentEnv) []MissedReceipt {
	t.Helper()
	tx, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("closing the read: %v", err)
		}
	}()
	all, err := OwedMissedReceiptsTx(context.Background(), tx, time.Time{}, 100)
	if err != nil {
		t.Fatalf("reading the owed receipts: %v", err)
	}
	var owed []MissedReceipt
	for _, missed := range all {
		if missed.SubjectID == e.contact.UUID {
			owed = append(owed, missed)
		}
	}
	return owed
}

// stopFailing records a stop for the lane's contact, who has no address, while
// alarm decides whether its missed receipt can be raised.
func stopFailing(t *testing.T, e *channelConsentEnv, alarm MissedReceipts) {
	t.Helper()
	owingAcknowledgement(t, e, true)
	e.store = e.store.WithMissedReceipts(alarm)
	if err := e.store.Suppress(e.ctx, SuppressInput{ContactID: e.contact, Kind: commsauthz.ReasonObjection}); err != nil {
		t.Fatalf("the stop was lost: %v", err)
	}
	var standing bool
	if err := e.owner.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM communication_suppression
		WHERE contact_id = $1 AND revoked_at IS NULL)`, e.contact).Scan(&standing); err != nil || !standing {
		t.Fatalf("the stop is not standing: %v", err)
	}
}

func TestAnAlarmThatCannotBeRaisedKeepsTheReceiptOwed(t *testing.T) {
	for name, alarm := range map[string]failingAlarm{
		"nobody to tell":   {},
		"a database error": {sql: `SELECT 1/0`},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupChannelConsent(t)
			stopFailing(t, e, alarm)
			var stop ids.UUID
			if err := e.owner.QueryRow(context.Background(),
				`SELECT id FROM communication_suppression WHERE contact_id = $1`, e.contact).Scan(&stop); err != nil {
				t.Fatal(err)
			}
			want := MissedReceipt{
				SubjectType: entityContact, SubjectID: e.contact.UUID,
				MessageID: acknowledgementMessageID(stop), Reason: MissedReceiptNoAddress,
			}
			if owed := owedReceipts(t, e); len(owed) != 1 || owed[0] != want {
				t.Fatalf("owed %+v, want only %+v", owed, want)
			}
		})
	}
}

// A raised alarm keeps nothing owed, and a notice under the receipt's key
// settles a kept one.
func TestOnlyAnUnraisedReceiptStaysOwed(t *testing.T) {
	e := setupChannelConsent(t)
	stopFailing(t, e, raisedAlarm{})
	if owed := owedReceipts(t, e); len(owed) != 0 {
		t.Fatalf("a raised alarm left %+v owed", owed)
	}

	e = setupChannelConsent(t)
	stopFailing(t, e, failingAlarm{})
	owed := owedReceipts(t, e)
	if len(owed) != 1 {
		t.Fatalf("owed %+v, want one", owed)
	}
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO notice (recipient_user_id, kind, subject, captured_by, dedupe_key)
		VALUES ($1, 'optout_receipt_missed', 'Receipt missed', 'system:test', $2)`,
		e.user, noticekeys.OptOutReceipt(owed[0].MessageID)); err != nil {
		t.Fatal(err)
	}
	if owed := owedReceipts(t, e); len(owed) != 0 {
		t.Fatalf("a raised receipt is still owed: %+v", owed)
	}
}

// A press is kept owed under its own message id, which nothing else records.
func TestAPressWhoseAlarmFailsKeepsItsOwnReceiptOwed(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	owingAcknowledgement(t, e, true)
	e.store = e.store.WithMissedReceipts(failingAlarm{})
	if _, err := e.store.Record(e.ctx, RecordInput{
		ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted", PolicyText: &grantWording,
	}); err != nil {
		t.Fatalf("granting before the press: %v", err)
	}
	if withdrawn, err := e.store.PublicStopAllMarketing(e.ctx, e.contact); err != nil || len(withdrawn) == 0 {
		t.Fatalf("the press: %v %v", withdrawn, err)
	}
	prefix := "optout-ack-" + strings.ReplaceAll(e.contact.String(), "-", "") + "-"
	if owed := owedReceipts(t, e); len(owed) != 1 || !strings.HasPrefix(owed[0].MessageID, prefix) {
		t.Fatalf("owed %+v, want one under the press's own id", owed)
	}
}

// A marker the database refuses is logged, and the stop still stands.
func TestAMarkerThatCannotBeWrittenLeavesTheStopStanding(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		CREATE FUNCTION refuse_owed_marker() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'refused by the test'; END $$;
		CREATE TRIGGER refuse_owed_marker BEFORE INSERT ON system_log FOR EACH ROW
		WHEN (NEW.action = 'optout_receipt_alarm_owed') EXECUTE FUNCTION refuse_owed_marker();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := e.owner.Exec(ctx, `DROP TRIGGER refuse_owed_marker ON system_log; DROP FUNCTION refuse_owed_marker()`); err != nil {
			t.Errorf("removing the test trigger: %v", err)
		}
	})
	stopFailing(t, e, failingAlarm{})
	if owed := owedReceipts(t, e); len(owed) != 0 {
		t.Fatalf("owed %+v, want the refused marker to leave none", owed)
	}
}
