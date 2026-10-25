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

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// waitForLockWaiter returns once a backend is blocked on a lock, so the
// test commits the competing write only after the update has reached its lock.
func waitForLockWaiter(ctx context.Context, t *testing.T, e *configEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		var waiting int
		if err := e.owner.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'`,
		).Scan(&waiting); err != nil {
			t.Fatalf("looking for a blocked update: %v", err)
		}
		if waiting > 0 {
			return
		}
	}
	t.Fatal("the update never blocked on a lock")
}

func TestAnUpdateRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := context.Background()
	stage := openStage(t, e, SemanticOpen)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(ctx, `SELECT pipeline_id FROM stage WHERE id = $1`, stage).Scan(&pipeline); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	newOwner := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'New owner')`,
		newOwner, "new-owner-"+newOwner.String()+"@config.test"); err != nil {
		t.Fatal(err)
	}
	formerOwner := ids.From[ids.UserKind](e.admin)
	deal, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Contested", PipelineID: pipeline, StageID: stage, OwnerID: &formerOwner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))

	rep := principal.WithActor(e.asDealWriter(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
		},
	})

	reassign, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reassign.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the reassignment: %v", err)
		}
	})
	if _, err := reassign.Exec(ctx, `UPDATE deal SET owner_id = $1 WHERE id = $2`, newOwner, dealID.UUID); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}

	name := "Renamed by the former owner"
	done := make(chan error, 1)
	go func() {
		_, err := e.store.UpdateDeal(rep, dealID, UpdateDealInput{Name: &name})
		done <- err
	}()
	waitForLockWaiter(ctx, t, e)
	if err := reassign.Commit(ctx); err != nil {
		t.Fatalf("committing the reassignment: %v", err)
	}

	if err := <-done; !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("former owner's update after the reassignment: %v, want a refusal", err)
	}
	var stored string
	if err := e.owner.QueryRow(ctx, `SELECT name FROM deal WHERE id = $1`, dealID.UUID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "Contested" {
		t.Fatalf("deal name is %q, the refused update must not have written", stored)
	}
}
