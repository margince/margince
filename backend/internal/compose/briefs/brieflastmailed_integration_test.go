// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

import (
	"testing"
	"time"
)

// The previous morning is the last claim before now that no failure followed.
// It sets where every morning's changes block starts.
func TestLastMailedBeforeSkipsAFailedClaimAndOneNotYetMade(t *testing.T) {
	b := setupBrief(t)
	first, second := briefClock, briefClock.AddDate(0, 0, 1)
	claim := func(at time.Time) BriefRun {
		t.Helper()
		run, err := b.engine.SnapshotRun(b.repCtx, at)
		if err != nil {
			t.Fatal(err)
		}
		if _, claimed, err := b.engine.ClaimMailAttempt(b.repCtx, run.ID, at); err != nil || !claimed {
			t.Fatalf("claiming the morning of %s = %v %v", at, claimed, err)
		}
		return run
	}
	lastBefore := func(now time.Time) *time.Time {
		t.Helper()
		at, err := b.engine.LastMailedBefore(b.repCtx, now)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}

	claim(first)
	if at := lastBefore(first); at != nil {
		t.Fatalf("a claim made at now reads %v, want it ignored", at)
	}
	failed := claim(second)
	if at := lastBefore(second); at == nil || !at.Equal(first) {
		t.Fatalf("just before the second claim = %v, want the first, %v", at, first)
	}
	if at := lastBefore(second.Add(time.Hour)); at == nil || !at.Equal(second) {
		t.Fatalf("after the second claim = %v, want %v", at, second)
	}
	if err := b.engine.MailFailed(b.repCtx, failed.ID, "the relay refused it"); err != nil {
		t.Fatal(err)
	}
	if at := lastBefore(second.Add(time.Hour)); at == nil || !at.Equal(first) {
		t.Fatalf("after the second claim failed = %v, want the first, %v", at, first)
	}
}
