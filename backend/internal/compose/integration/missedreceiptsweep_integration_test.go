// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The controller payload sweep raises the missed receipts whose alarm failed:
// every one of them, never one about a subject an erasure has taken.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// owedReceiptCode is a code no shipped pack holds, owing a receipt.
const owedReceiptCode jurisdiction.Code = "qr"

var registerOwedReceiptRules sync.Once

type nobodyToTell struct{}

func (nobodyToTell) RaiseMissedReceiptTx(context.Context, pgx.Tx, consent.MissedReceipt) error {
	return errors.New("nobody can be told")
}

// owedReceiptEnv is an installation whose administrator is the privacy
// operator the sweep tells.
func owedReceiptEnv(t *testing.T) *Env {
	t.Helper()
	registerOwedReceiptRules.Do(func() {
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: owedReceiptCode, Version: 1, OptOutAcknowledgement: true, OptOutAcknowledgementWithin: 24 * time.Hour,
		})
	})
	e := Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment (user_id, role_id) SELECT $1, id FROM role WHERE key = 'admin'`, e.AdminUser)
	return e
}

// owedReceipt stops a new contact while its alarm cannot be raised, and answers
// the contact and the row that keeps its receipt owed.
func owedReceipt(t *testing.T, e *Env, name string) (contact, marker ids.UUID) {
	t.Helper()
	contact = e.SeedContact(t, name, &e.AdminUser)
	e.WsExec(t, `UPDATE contact SET address_country = upper($2) WHERE id = $1`, contact, string(owedReceiptCode))
	store := consent.NewStore(compose.InstallationDB(e.Pool)).WithMissedReceipts(nobodyToTell{})
	if err := store.Suppress(e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: commsauthz.ReasonObjection,
	}); err != nil {
		t.Fatalf("stopping %s: %v", name, err)
	}
	if err := e.Pool.QueryRow(context.Background(), `SELECT id FROM system_log
		WHERE action = 'optout_receipt_alarm_owed' AND detail->>'subject_id' = $1`, contact.String()).Scan(&marker); err != nil {
		t.Fatalf("no owed receipt for %s: %v", name, err)
	}
	return contact, marker
}

// copyOwed keeps n more receipts owed, each a copy of marker under its own
// message id. Their row ids come from the clock the marker writer uses, so they
// sort before every marker written after them.
func copyOwed(t *testing.T, e *Env, marker ids.UUID, n int) {
	t.Helper()
	rows := make([]ids.UUID, n)
	for i := range rows {
		rows[i] = ids.NewV7()
	}
	if _, err := e.Pool.Exec(context.Background(), `
		INSERT INTO system_log (id, actor_type, actor_id, action, detail)
		SELECT copy.id, l.actor_type, l.actor_id, l.action,
		       jsonb_set(l.detail, '{message_id}', to_jsonb((l.detail->>'message_id') || '-' || copy.n))
		  FROM system_log l, unnest($2::uuid[]) WITH ORDINALITY AS copy(id, n) WHERE l.id = $1`, marker, rows); err != nil {
		t.Fatalf("copying the owed receipt: %v", err)
	}
}

// sweepOwed sweeps at the database's own now, the clock the owed rows carry.
func sweepOwed(t *testing.T, e *Env) error {
	t.Helper()
	return compose.DriveControllerPayloadSweepForTest(context.Background(), e.Pool, keyvault.NewMemory(), databaseNow(t, e))
}

func databaseNow(t *testing.T, e *Env) time.Time {
	t.Helper()
	var now time.Time
	if err := e.Pool.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now
}

func raisedAbout(t *testing.T, e *Env, contact ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice
		WHERE kind = 'optout_receipt_missed' AND target_id = $1 AND retracted_at IS NULL`, contact)
}

// One sweep reads past its first page.
func TestOneSweepRaisesMoreOwedReceiptsThanAPageHolds(t *testing.T) {
	e := owedReceiptEnv(t)
	contact, marker := owedReceipt(t, e, "Paula Page")
	copyOwed(t, e, marker, 200)
	if err := sweepOwed(t, e); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if n := raisedAbout(t, e, contact); n != 201 {
		t.Fatalf("%d of 201 owed receipts raised", n)
	}
}

// Receipts owed about an erased subject never settle, and must never hold back
// a receipt owed after them.
func TestReceiptsOwedAboutAnErasedSubjectDoNotHoldBackALaterOne(t *testing.T) {
	e := owedReceiptEnv(t)
	erased, marker := owedReceipt(t, e, "Erik Erased")
	copyOwed(t, e, marker, 2000)
	e.WsExec(t, `UPDATE contact SET full_name = 'Erased Subject', archived_at = now() WHERE id = $1`, erased)
	live, _ := owedReceipt(t, e, "Lena Later")
	if err := sweepOwed(t, e); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if n := raisedAbout(t, e, live); n != 1 {
		t.Fatalf("%d notices for the receipt owed after 2001 about an erased subject, want 1", n)
	}
}

// An erasure that has changed the subject but not yet taken back its notices
// makes the sweep wait. The sweep then raises nothing about the subject.
func TestTheSweepWaitsForAnErasureInFlightAndRaisesNothing(t *testing.T) {
	e := owedReceiptEnv(t)
	contact, _ := owedReceipt(t, e, "Hanna Held")
	ctx := context.Background()
	erasure, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := erasure.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("closing the erasure: %v", err)
		}
	}()
	var erasurePID int32
	if err := erasure.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&erasurePID); err != nil {
		t.Fatal(err)
	}
	if _, err := erasure.Exec(ctx, `UPDATE contact SET full_name = 'Erased Subject' WHERE id = $1`, contact); err != nil {
		t.Fatal(err)
	}

	at := databaseNow(t, e)
	swept := make(chan struct{})
	var sweepErr error
	go func() {
		defer close(swept)
		sweepErr = compose.DriveControllerPayloadSweepForTest(ctx, e.Pool, keyvault.NewMemory(), at)
	}()
	testdb.AwaitQueuedBehind(t, OwnerConn(t), erasurePID, 1, swept, "the sweep")
	if err := erasure.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	<-swept
	if sweepErr != nil {
		t.Fatalf("sweeping: %v", sweepErr)
	}
	if n := raisedAbout(t, e, contact); n != 0 {
		t.Fatalf("%d notices raised about a subject erased under the sweep", n)
	}
}
