// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Wrong second-factor codes count toward the account lockout the password
// shares. So the six-digit code has a durable per-account cap that neither a
// reused challenge nor a burst of parallel requests gets past.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

func TestWrongSecondFactorCodesLockTheAccount(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-lockout")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	secret := e.enrolMemberMFA(t)
	e.atInstant(base.Add(totpStep))

	// Every guess rides one challenge: the cap is the account's, not the nonce's.
	jti := freshJti()
	for attempt := 1; attempt <= lockoutThreshold; attempt++ {
		if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, jti, "000000"); !errors.Is(err, errBadMFACode) {
			t.Fatalf("wrong code %d: err=%v, want errBadMFACode", attempt, err)
		}
	}
	if n := mfaFailureRows(t, e); n != lockoutThreshold {
		t.Errorf("mfa_challenge_failed rows = %d, want %d", n, lockoutThreshold)
	}

	// Locked: even the right code, on a fresh challenge, opens no session.
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, token, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, freshJti(), code); !errors.Is(err, errAccountLocked) || token != "" {
		t.Fatalf("right code while locked: err=%v token=%q, want errAccountLocked and no session", err, token)
	}

	// Once the lock lapses the right code signs in and clears the streak.
	e.atInstant(base.Add(lockoutDuration + 2*totpStep))
	code, _ = totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, freshJti(), code); err != nil {
		t.Fatalf("right code after the lock lapsed: %v", err)
	}
	if count, lockedUntil := lockoutColumns(t, e); count != 0 || lockedUntil != nil {
		t.Errorf("a completed sign-in left the streak at count=%d locked_until=%v, want it reset", count, lockedUntil)
	}
}

func TestAMistypedCodeCanBeRetypedOnTheSameChallenge(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-retype")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	secret := e.enrolMemberMFA(t)
	e.atInstant(base.Add(totpStep))

	jti := freshJti()
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, jti, "000000"); !errors.Is(err, errBadMFACode) {
		t.Fatalf("wrong code: err=%v, want errBadMFACode", err)
	}
	if count, _ := lockoutColumns(t, e); count != 1 {
		t.Errorf("failed_login_count after one wrong code = %d, want 1", count)
	}
	code, _ := totpCodeAt(secret, e.svc.now(), totpDigits)
	if _, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, jti, code); err != nil {
		t.Fatalf("right code on the same challenge after a typo: %v", err)
	}
}

func TestConcurrentWrongCodesCannotOutrunTheLockout(t *testing.T) {
	e := setupRevocationEnv(t, "mfa-lockout-burst")
	e.withVault()
	base := time.Unix(1_760_000_000, 0)
	e.atInstant(base)
	e.enrolMemberMFA(t)
	e.atInstant(base.Add(totpStep))

	// Each attempt queues on the member row. The first lockoutThreshold of them
	// are judged, and every later one finds the account already locked.
	const burst = 3 * lockoutThreshold
	results := make(chan error, burst)
	var wg sync.WaitGroup
	for range burst {
		wg.Go(func() {
			_, _, err := e.svc.CompleteMFAChallenge(e.wsOnlyCtx(), e.member.UserID, freshJti(), "000000")
			results <- err
		})
	}
	wg.Wait()
	close(results)

	wrong, locked := 0, 0
	for err := range results {
		switch {
		case errors.Is(err, errBadMFACode):
			wrong++
		case errors.Is(err, errAccountLocked):
			locked++
		default:
			t.Fatalf("burst attempt: err=%v, want errBadMFACode or errAccountLocked", err)
		}
	}
	if wrong != lockoutThreshold || locked != burst-lockoutThreshold {
		t.Errorf("burst of %d judged %d codes and refused %d as locked, want %d and %d",
			burst, wrong, locked, lockoutThreshold, burst-lockoutThreshold)
	}
}

func mfaFailureRows(t *testing.T, e *revocationEnv) int {
	t.Helper()
	var n int
	if err := database.WithInfraTx(context.Background(), e.svc.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM system_log WHERE action = 'mfa_challenge_failed' AND actor_id = $1`,
			"human:"+e.member.UserID.String()).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func lockoutColumns(t *testing.T, e *revocationEnv) (int, *time.Time) {
	t.Helper()
	var count int
	var lockedUntil *time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT failed_login_count, locked_until FROM app_user WHERE id = $1`,
		e.member.UserID).Scan(&count, &lockedUntil); err != nil {
		t.Fatal(err)
	}
	return count, lockedUntil
}
