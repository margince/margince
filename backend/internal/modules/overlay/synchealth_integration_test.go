// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package overlay

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/overlaybudget"
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

	// A freshly connected workspace has not imported anything yet, and the
	// read says so: the initial import still owed is the one honest concern,
	// never a clean bill of health for data that was never brought over.
	concerns, err := svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth on a fresh connection: %v", err)
	}
	if len(concerns) != 1 || concerns[0].Kind != ConcernBackfillIncomplete {
		t.Fatalf("a freshly connected workspace reports %v, want exactly the still-owed import", concerns)
	}

	if err := store.RecordSweepFailure(ctx, apperrors.ErrIncumbentBudgetExhausted); err != nil {
		t.Fatalf("RecordSweepFailure: %v", err)
	}
	concerns, err = svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth after a sweep failure: %v", err)
	}
	failing, laddered := concernOfKind(concerns, ConcernSyncFailing)
	if !laddered {
		t.Fatalf("after a sweep failure the read reports %v, want a %s concern", concerns, ConcernSyncFailing)
	}
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
	if _, still := concernOfKind(concerns, ConcernSyncFailing); still {
		t.Fatalf("one clean sweep must clear the failing concern, still reports %v", concerns)
	}
}

// concernOfKind picks the named concern out of a reading, so an assertion
// about one condition stays true as other honest conditions come and go.
func concernOfKind(concerns []SyncConcern, kind string) (SyncConcern, bool) {
	for _, c := range concerns {
		if c.Kind == kind {
			return c, true
		}
	}
	return SyncConcern{}, false
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
	// The sibling arm that makes "the same refusal every /overlay op does" a
	// held claim rather than a comment: Budget refuses this workspace with
	// the identical sentinel.
	if _, err := svc.WithBudgetMeter(overlaybudget.New(nil, nil)).Budget(ctx); !errors.Is(err, apperrors.ErrModeNotOverlay) {
		t.Fatalf("Budget on the same workspace = %v, want the same ErrModeNotOverlay", err)
	}
}
