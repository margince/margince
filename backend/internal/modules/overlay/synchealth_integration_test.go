// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package overlay

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The sync-health read follows the ladder the poller actually writes: a sweep
// failure raises the concern, a clean sweep clears it — seeded through
// RecordSweepFailure/RecordSweepSuccess, the same writers the worker calls.
func TestSyncHealthFollowsTheSweepLadder(t *testing.T) {
	ctx, pool, ws := testWorkspaceCtx(t)
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	store := NewMirrorStore(db, noOwnerEmails{})
	svc := NewService(db, keyvault.NewMemory(), store)
	if _, err := svc.Connect(ctx, ConnectInput{Incumbent: "hubspot", Region: "eu1", Token: "tok"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	concerns, err := svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth on a fresh connection: %v", err)
	}
	if len(concerns) != 0 {
		t.Fatalf("a freshly connected workspace reports %v, want no concerns", concerns)
	}

	if err := store.RecordSweepFailure(ctx, apperrors.ErrIncumbentBudgetExhausted); err != nil {
		t.Fatalf("RecordSweepFailure: %v", err)
	}
	concerns, err = svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth after a sweep failure: %v", err)
	}
	if len(concerns) != 1 || concerns[0].Kind != ConcernSyncFailing {
		t.Fatalf("after a sweep failure the read reports %v, want one %s concern", concerns, ConcernSyncFailing)
	}
	failing := concerns[0]
	if failing.ErrorClass != string(classSweepRateLimited) || failing.Failures != 1 {
		t.Errorf("the concern carries class %q / %d failures, want %q / 1 — the ladder's own facts",
			failing.ErrorClass, failing.Failures, classSweepRateLimited)
	}
	if failing.NextSweepAt == nil {
		t.Error("the concern names no retry time although the ladder paced one")
	}

	if err := store.RecordSweepSuccess(ctx); err != nil {
		t.Fatalf("RecordSweepSuccess: %v", err)
	}
	concerns, err = svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth after a clean sweep: %v", err)
	}
	if len(concerns) != 0 {
		t.Fatalf("one clean sweep must clear the concern, still reports %v", concerns)
	}
}

// A workspace that never connected an incumbent has no sync to be healthy
// about: the read answers the same mode refusal every /overlay op does, which
// the feed renders as an absent lane rather than a clear bill of health.
func TestSyncHealthRefusesANativeModeWorkspace(t *testing.T) {
	ctx, pool, ws := testWorkspaceCtx(t) // never flips to overlay mode
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	svc := NewService(db, keyvault.NewMemory(), NewMirrorStore(db, noOwnerEmails{}))

	if _, err := svc.SyncHealth(ctx); !errors.Is(err, apperrors.ErrModeNotOverlay) {
		t.Fatalf("SyncHealth on a native-mode workspace = %v, want ErrModeNotOverlay", err)
	}
}
