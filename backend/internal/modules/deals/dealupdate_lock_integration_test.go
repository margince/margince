// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// waitUntilBlockedBy returns once a backend waits on a lock held by blocker,
// so the competing commit happens only after the writer reached its lock.
func waitUntilBlockedBy(ctx context.Context, t *testing.T, e *configEnv, blocker int) {
	t.Helper()
	for ctx.Err() == nil {
		var waiting bool
		if err := e.owner.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY (pg_blocking_pids(pid)))`, blocker,
		).Scan(&waiting); err != nil {
			t.Fatalf("looking for a writer blocked by the reassignment: %v", err)
		}
		if waiting {
			return
		}
	}
	t.Fatal("no writer ever blocked on the reassigning transaction")
}

// asFormerOwner holds every deal verb over the caller's OWN deals only.
func (e *configEnv) asFormerOwner() context.Context {
	return principal.WithActor(e.asDealWriter(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true, Delete: true}},
		},
	})
}

// seedOwnedDeal creates an open deal owned by the admin user and answers the
// deal with the ids of two stages of its pipeline.
func seedOwnedDeal(t *testing.T, e *configEnv) (deal ids.DealID, from, to ids.StageID) {
	t.Helper()
	from = openStage(t, e, SemanticOpen)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(context.Background(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipeline); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	probability := 10
	next, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipeline, Name: "Proposal", Position: 1, Semantic: string(SemanticOpen), WinProbability: &probability,
	})
	if err != nil {
		t.Fatalf("seeding the second stage: %v", err)
	}
	owner := ids.From[ids.UserKind](e.admin)
	created, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Contested", PipelineID: pipeline, StageID: from, OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	return ids.From[ids.DealKind](ids.UUID(created.Id)), from, ids.From[ids.StageKind](ids.UUID(next.Id))
}

// refusedAfterReassignment runs write as the deal's former owner while an
// uncommitted transaction reassigns the deal, commits the reassignment once
// the write waits on it, and answers what the write returned.
func refusedAfterReassignment(t *testing.T, e *configEnv, deal ids.DealID, write func(ctx context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	newOwner := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'New owner')`,
		newOwner, "new-owner-"+newOwner.String()+"@config.test"); err != nil {
		t.Fatal(err)
	}
	reassign, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reassign.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the reassignment: %v", err)
		}
	})
	var blocker int
	if err := reassign.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := reassign.Exec(ctx, `UPDATE deal SET owner_id = $1 WHERE id = $2`, newOwner, deal.UUID); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- write(e.asFormerOwner()) }()
	waitUntilBlockedBy(ctx, t, e, blocker)
	if err := reassign.Commit(ctx); err != nil {
		t.Fatalf("committing the reassignment: %v", err)
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatal("the write never returned after the reassignment committed")
		return nil
	}
}

func requireRefusal(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("%s by the former owner after the reassignment: %v, want a refusal", what, err)
	}
}

func TestAnUpdateRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	name := "Renamed by the former owner"

	err := refusedAfterReassignment(t, e, deal, func(ctx context.Context) error {
		_, err := e.store.UpdateDeal(ctx, deal, UpdateDealInput{Name: &name})
		return err
	})

	requireRefusal(t, "an update", err)
	var stored string
	if err := e.owner.QueryRow(context.Background(), `SELECT name FROM deal WHERE id = $1`, deal.UUID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "Contested" {
		t.Fatalf("deal name is %q, the refused update must not have written", stored)
	}
}

func TestAnAdvanceRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	deal, from, to := seedOwnedDeal(t, e)

	err := refusedAfterReassignment(t, e, deal, func(ctx context.Context) error {
		_, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: to})
		return err
	})

	requireRefusal(t, "an advance", err)
	var stage ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT stage_id FROM deal WHERE id = $1`, deal.UUID).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if stage != from.UUID {
		t.Fatalf("deal moved to stage %s, the refused advance must not have written", stage)
	}
}

func TestAnArchiveRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)

	err := refusedAfterReassignment(t, e, deal, func(ctx context.Context) error {
		_, err := e.store.ArchiveDeal(ctx, deal, nil)
		return err
	})

	requireRefusal(t, "an archive", err)
	var archived bool
	if err := e.owner.QueryRow(context.Background(), `SELECT archived_at IS NOT NULL FROM deal WHERE id = $1`, deal.UUID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived {
		t.Fatal("the refused archive still archived the deal")
	}
}

// noLinksRestore is what an un-archive of a deal with no lists or tags needs:
// the erasure check and link restorers are never asked about anything.
var noLinksRestore = storekit.RestoreWith{
	Erased: func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return false, nil },
	Links: storekit.LinkRestore{
		Membership: func(context.Context, pgx.Tx, string, ids.UUID, storekit.ListMembership) (bool, error) {
			return false, nil
		},
		Tag: func(context.Context, pgx.Tx, string, ids.UUID, storekit.TagAssignment) (bool, error) {
			return false, nil
		},
	},
}

func TestARestoreRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	if _, err := e.store.ArchiveDeal(e.asFormerOwner(), deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	err := refusedAfterReassignment(t, e, deal, func(ctx context.Context) error {
		return e.store.Tx(ctx, func(tx pgx.Tx) error {
			_, err := e.store.RestoreDealTx(ctx, tx, deal, nil, noLinksRestore)
			return err
		})
	})

	requireRefusal(t, "a restore", err)
}

// seedCorrection writes a close-date correction through the writers the sweep
// uses: the patch, its audit row and the correction row, in one transaction.
func seedCorrection(t *testing.T, e *configEnv, deal ids.DealID) ids.UUID {
	t.Helper()
	var correction ids.UUID
	err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		ctx := e.asDealWriter()
		patch := storekit.NewPatch()
		patch.Set("expected_close_date", nil, "2031-01-02")
		if err := applyDealPatchGuarded(ctx, tx, deal, patch, nil); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", deal.UUID, patch.Before(), patch.After())
		if err != nil {
			return err
		}
		recorded, err := recordCorrection(ctx, tx, DealCorrection{
			DealID: deal, AuditLogID: auditID, Correction: "auto_apply", Fields: movedFields(patch),
			Evidence: CorrectionEvidence{RemainingOpenStages: "1", Asking: "a", StandingCloseDate: "b"},
		}, nil)
		correction = recorded.ID
		return err
	})
	if err != nil {
		t.Fatalf("seeding the correction: %v", err)
	}
	return correction
}

func TestACorrectionRevertRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	correction := seedCorrection(t, e, deal)

	err := refusedAfterReassignment(t, e, deal, func(ctx context.Context) error {
		_, err := e.store.RevertCorrection(ctx, correction, nil, nil)
		return err
	})

	requireRefusal(t, "a correction revert", err)
	var reversed bool
	if err := e.owner.QueryRow(context.Background(), `SELECT reversed_at IS NOT NULL FROM deal_correction WHERE id = $1`, correction).Scan(&reversed); err != nil {
		t.Fatal(err)
	}
	if reversed {
		t.Fatal("the refused revert still stamped the correction as taken back")
	}
}
