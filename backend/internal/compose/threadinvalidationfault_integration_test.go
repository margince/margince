// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAFailedPrivacyRepairKeepsItsBarrierAndLetsOtherConversationsFinish(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	basis := seedThreadPrivacyGround(t, f, &f.Erased, "Erased source proof")
	stageThreadPrivacyRepair(t, f, false)
	eraseThreadPrivacySource(t, f)
	apply := threadRepairApplier(f.Pool)
	actual := apply.Invalidate
	apply.Invalidate = func(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
		changed, err := actual(ctx, tx, id)
		if err != nil {
			return changed, err
		}
		return false, errors.New("injected privacy owner fault")
	}
	run := ids.NewV7()
	for attempt := 1; attempt <= 3; attempt++ {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			scanned, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, apply)
			if scanned != 1 {
				t.Fatalf("privacy unit count=%d", scanned)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if got := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation WHERE activity_id=$1 AND failures=$2`, f.Erased, attempt); got != 1 {
			t.Fatalf("attempt%d was not persisted", attempt)
		}
		if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE id=$1 AND revoked_at IS NULL AND note='Erased source proof'`, basis); got != 1 {
			t.Fatal("failed owner mutation escaped its savepoint")
		}
		if attempt < 3 {
			f.WsExec(t, `UPDATE capture_thread_invalidation SET retry_after='2000-01-01' WHERE activity_id=$1`, f.Erased)
		}
	}
	assertThreadRepairHealth(t, f.Env, 1)
	const other = "unrelated-to-privacy-fault@other.example"
	raw := subjectThreadMail(seatAddress(t, f.Env, f.Rep1), other, "Another discussion", "")
	raw = []byte(strings.Replace(string(raw), "From: Pat Counterparty <pat@counterparty.example>", "From: Sam Correspondent <sam@other.example>", 1))
	captureThreadMail(t, f.Env, f.Rep1, raw)
	available := false
	for range 100 {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 32, run, threadRepairApplier(f.Pool)); err != nil {
				return err
			}
			return tx.QueryRow(f.Admin(), `SELECT activity_thread_available(@key)`, pgx.NamedArgs{"key": other}).Scan(&available)
		}); err != nil {
			t.Fatal(err)
		}
		if available {
			break
		}
	}
	if !available {
		t.Fatal("unrelated conversation could not publish")
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		var hidden bool
		if err := tx.QueryRow(f.Admin(), `SELECT NOT activity_thread_available(@key)`, pgx.NamedArgs{"key": f.Root}).Scan(&hidden); err != nil {
			return err
		}
		if !hidden {
			t.Fatal("failed privacy repair became visible")
		}
		out, err := repairThreadsWithApplier(principal.SystemActing(f.Admin(), "system:test_privacy_correction"), tx, datatask.BatchIn{Limit: 256, RunID: run}, threadRepairApplier(f.Pool))
		if out.Done {
			t.Fatalf("correction claimed completion: %+v %v", out, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(dataTaskReader(f.Env), f.Pool, func(tx pgx.Tx) error { return capture.RetryThreadRepairTx(dataTaskReader(f.Env), tx) }); err != nil {
		t.Fatal(err)
	}
	assertThreadRepairHealth(t, f.Env, 0)
	finishThreadRepairs(t, f.Env)
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE id=$1 AND revoked_at IS NOT NULL AND note IS NULL`, basis); got != 1 {
		t.Fatal("retried owner invalidation did not finish")
	}
	if got := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation`); got != 0 {
		t.Fatalf("retry left%d inbox rows", got)
	}
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
		t.Fatalf("survivor retained erased parent: %s", got)
	}
}
