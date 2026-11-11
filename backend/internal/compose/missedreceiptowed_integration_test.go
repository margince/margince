// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The controller payload sweep raises a missed receipt whose alarm failed.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// failedReceiptAlarm records a refusal owing a receipt to a contact with no
// address while nobody can be told. It answers when the receipt was kept as
// owed, on the database clock, and how to bring the operator back.
func failedReceiptAlarm(t *testing.T) (l *confirmLaneEnv, owedAt time.Time, restore func()) {
	t.Helper()
	l = receiptLane(t, receiptOwed)
	l.e.WsExec(t, `DELETE FROM contact_email WHERE contact_id = $1`, l.contact)
	l.e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1 AND role_id IN (SELECT id FROM role WHERE key = 'admin')`, l.e.AdminUser)
	l.object(t)
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT occurred_at FROM system_log
		WHERE action = 'optout_receipt_alarm_owed' AND detail->>'subject_id' = $1`, l.contact.String()).Scan(&owedAt); err != nil {
		t.Fatalf("the failed alarm kept no owed receipt: %v", err)
	}
	return l, owedAt, func() {
		l.e.WsExec(t, `INSERT INTO role_assignment (user_id, role_id) SELECT $1, id FROM role WHERE key = 'admin'`, l.e.AdminUser)
	}
}

func (l *confirmLaneEnv) sweepAt(t *testing.T, at time.Time) int {
	t.Helper()
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, keyvault.NewMemory(), at); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	return len(l.missedReceiptNotices(t))
}

// The sweep raises nothing while nobody can be told, then raises the receipt
// once an operator is back, and only once.
func TestAMissedReceiptWhoseAlarmFailedIsRaisedByTheNextSweep(t *testing.T) {
	l, owedAt, restore := failedReceiptAlarm(t)
	at := owedAt.Add(time.Hour)
	if n := l.sweepAt(t, at); n != 0 {
		t.Fatalf("%d notices while nobody could be told", n)
	}
	restore()
	if n := l.sweepAt(t, at); n != 1 {
		t.Fatalf("%d notices after the operator came back, want 1", n)
	}
	if n := l.sweepAt(t, at); n != 1 {
		t.Fatalf("%d notices after a second sweep, want still 1", n)
	}
}

func TestAnOwedReceiptIsRaisedForThirtyDaysOnly(t *testing.T) {
	l, owedAt, restore := failedReceiptAlarm(t)
	restore()
	if n := l.sweepAt(t, owedAt.Add(31*24*time.Hour)); n != 0 {
		t.Fatalf("%d notices for a receipt owed 31 days ago", n)
	}
	if n := l.sweepAt(t, owedAt.Add(29*24*time.Hour)); n != 1 {
		t.Fatalf("%d notices for a receipt owed 29 days ago, want 1", n)
	}
}

func TestAnOwedReceiptAboutAnErasedSubjectIsNotRaised(t *testing.T) {
	l, owedAt, restore := failedReceiptAlarm(t)
	if err := ComposedEraser(l.e.Pool, nil, nil).EraseContact(l.e.Admin(), l.contact, "Art. 17 request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	restore()
	l.sweepAt(t, owedAt.Add(time.Hour))
	if n := l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed'`); n != 0 {
		t.Fatalf("%d notices raised about an erased subject", n)
	}
}
