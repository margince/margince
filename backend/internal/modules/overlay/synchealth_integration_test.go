// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package overlay

import (
	"errors"
	"slices"
	"testing"
	"time"

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

// An overwrite the sweep already committed reaches the lane, aggregated to the
// CLASSES to go and check rather than one card per row — and ages out of it,
// because an overwrite is a past act that never stops being true and a lane
// carrying every one forever is a log, not a prompt.
//
// Driven through Reconcile rather than by planting a system_log row: the lane
// reads back what the sweep writes, and a test that wrote its own row would
// keep passing if the two ever spelled the ledger differently.
func TestSyncHealthReportsRecentlyOverwrittenClassesAndThenForgetsThem(t *testing.T) {
	ctx, pool, ws := testWorkspaceCtx(t)
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	store := NewMirrorStore(db, noOwnerEmails{})
	svc := NewService(db, keyvault.NewMemory(), store)
	if _, err := svc.Connect(ctx, ConnectInput{Incumbent: "hubspot", Region: "eu1", Token: "tok"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	const objectClass = "organization"
	oldBaseline := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	// Two rows of the same class, so the reading proves the aggregation and
	// not merely that one conflict arrives.
	for _, externalID := range []string{"61655665850", "61655665851"} {
		if err := store.Ingest(ctx, Record{
			ObjectClass: objectClass, ExternalID: externalID,
			Fields:     map[string]any{"display_name": "Edited Here"},
			ModifiedAt: oldBaseline,
		}); err != nil {
			t.Fatalf("seeding the pre-existing mirror row: %v", err)
		}
		inc := &sweptRecords{records: []Record{{
			ObjectClass: objectClass, ExternalID: externalID,
			Fields:     map[string]any{"display_name": "Overwritten From The Incumbent"},
			ModifiedAt: oldBaseline.Add(time.Hour),
		}}}
		if _, err := Reconcile(ctx, inc, store, testBudgetMeter(t, "test-overwrite"),
			objectClass, oldBaseline.Add(-time.Second), oldBaseline); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}

	// A third overwrite, of a different class, older than the window the lane
	// looks through. Planted rather than swept because system_log is
	// append-only — no UPDATE can age a row the sweep wrote — and because what
	// is under test here is the READ's horizon, not the writer.
	if _, err := pool.Exec(ctx, `
		INSERT INTO system_log (actor_type, actor_id, action, detail, occurred_at)
		VALUES ('system', 'system:reconcile', $1, jsonb_build_object('object_class', 'person'),
		        now() - $2::interval)`,
		mirrorConflictAction, overwriteWindow+time.Hour); err != nil {
		t.Fatalf("planting the aged overwrite: %v", err)
	}

	concerns, err := svc.SyncHealth(ctx)
	if err != nil {
		t.Fatalf("SyncHealth after two overwrites: %v", err)
	}
	overwritten, reported := concernOfKind(concerns, ConcernRecordsOverwritten)
	if !reported {
		t.Fatalf("two overwritten rows report %v, want a %s concern", concerns, ConcernRecordsOverwritten)
	}
	// One entry, not two, and not three: two rows of one class are one class,
	// and the class overwritten before the window opened is not news.
	if !slices.Equal(overwritten.Objects, []string{objectClass}) {
		t.Errorf("the concern names %v, want exactly [%s]", overwritten.Objects, objectClass)
	}
}
