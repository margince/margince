// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// A claim whose create never landed is reclaimable; one whose create did land
// is not.
//
// This is the half that cannot be proven at unit speed. The claim's two states
// live in a column, the lease is measured by the database's own clock, and the
// crash the lease exists for is "the row is here and the record is not" — which
// only a real table can hold.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ageClaim backdates a claim so the lease reads it as abandoned, without the
// test waiting fifteen real minutes for it. The clock stays the database's —
// only the row moves.
func ageClaim(t *testing.T, fx *autoFixture, handler string) {
	t.Helper()
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	if err := db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE automation_effect_claim SET created_at = now() - interval '1 hour' WHERE handler = $1`, handler)
		return err
	}); err != nil {
		t.Fatalf("ageing the claim: %v", err)
	}
}

func claimState(t *testing.T, fx *autoFixture, handler string) (rows int, applied bool) {
	t.Helper()
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	if err := db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*), coalesce(bool_or(applied_at IS NOT NULL), false)
			   FROM automation_effect_claim WHERE handler = $1`, handler).Scan(&rows, &applied)
	}); err != nil {
		t.Fatalf("reading the claim: %v", err)
	}
	return rows, applied
}

// The crash this exists for: a worker took the claim and died before its create
// landed. A sibling firing past the lease must be able to apply it, because
// nothing else ever will — the occurrence has already happened.
func TestAClaimStrandedByACrashIsReclaimedAfterItsLease(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	claims := NewEffectClaims(db)
	ctx := context.Background()
	const handler, occurrence, fingerprint = "route_lead", "occurrence-1", "fp-1"

	// The firing that died: it took the claim and never confirmed.
	taken, err := claims.Claim(ctx, handler, occurrence, fingerprint)
	if err != nil || !taken {
		t.Fatalf("first claim: taken=%v err=%v", taken, err)
	}
	// While it is fresh, a sibling must NOT steal it — that firing may still be
	// working, and taking it over would write the record twice.
	if stolen, err := claims.Claim(ctx, handler, occurrence, fingerprint); err != nil || stolen {
		t.Fatalf("a fresh unapplied claim was stolen: stolen=%v err=%v", stolen, err)
	}

	ageClaim(t, fx, handler)

	reclaimed, err := claims.Claim(ctx, handler, occurrence, fingerprint)
	if err != nil {
		t.Fatalf("reclaiming: %v", err)
	}
	if !reclaimed {
		t.Fatal("a claim stranded past its lease was not reclaimable — the task it guards is lost forever")
	}
	if rows, _ := claimState(t, fx, handler); rows != 1 {
		t.Errorf("reclaiming left %d claim rows, want 1 — a takeover must not mint a second", rows)
	}
}

// A claim whose create LANDED is never reclaimed, however old it gets. This is
// the direction that must not fail: reclaiming an applied claim writes the
// record a second time, which is the corruption the claim exists to stop.
func TestAnAppliedClaimIsNeverReclaimedHoweverOld(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	claims := NewEffectClaims(db)
	ctx := context.Background()
	const handler, occurrence, fingerprint = "check_in_cadence", "occurrence-2", "fp-2"

	if taken, err := claims.Claim(ctx, handler, occurrence, fingerprint); err != nil || !taken {
		t.Fatalf("first claim: taken=%v err=%v", taken, err)
	}
	if err := claims.Confirm(ctx, handler, occurrence, fingerprint); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if _, applied := claimState(t, fx, handler); !applied {
		t.Fatal("Confirm did not mark the claim applied")
	}

	ageClaim(t, fx, handler)

	if stolen, err := claims.Claim(ctx, handler, occurrence, fingerprint); err != nil || stolen {
		t.Fatalf("an applied claim was reclaimed after ageing: stolen=%v err=%v — the record would be written twice", stolen, err)
	}
}

// Confirm is idempotent about a claim somebody else already applied: it touches
// only the unapplied row, so a late confirm from a reclaimed firing cannot
// overwrite the applied_at of the firing that actually landed the record.
func TestConfirmingAnAlreadyAppliedClaimChangesNothing(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	claims := NewEffectClaims(db)
	ctx := context.Background()
	const handler, occurrence, fingerprint = "no_activity_reminder", "occurrence-3", "fp-3"

	if taken, err := claims.Claim(ctx, handler, occurrence, fingerprint); err != nil || !taken {
		t.Fatalf("first claim: taken=%v err=%v", taken, err)
	}
	if err := claims.Confirm(ctx, handler, occurrence, fingerprint); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	if err := claims.Confirm(ctx, handler, occurrence, fingerprint); err != nil {
		t.Fatalf("a second confirm must not error: %v", err)
	}

	rows, applied := claimState(t, fx, handler)
	if rows != 1 || !applied {
		t.Errorf("after two confirms: rows=%d applied=%v, want 1 and true", rows, applied)
	}
}

// A genuinely different effect fingerprints apart and takes its own claim —
// the property the claim had before this change, asserted again because the
// ON CONFLICT clause that carries the lease is the one that could lose it.
func TestADifferentFingerprintStillTakesItsOwnClaim(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	claims := NewEffectClaims(db)
	ctx := context.Background()
	const handler, occurrence = "route_lead", "occurrence-4"

	first, err := claims.Claim(ctx, handler, occurrence, "fp-a")
	if err != nil || !first {
		t.Fatalf("first: taken=%v err=%v", first, err)
	}
	second, err := claims.Claim(ctx, handler, occurrence, "fp-b")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second {
		t.Fatal("a different fingerprint folded against another effect's claim")
	}
	if rows, _ := claimState(t, fx, handler); rows != 2 {
		t.Errorf("two different effects left %d claim rows, want 2", rows)
	}
}
