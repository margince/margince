// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type pausedMemberInspection struct {
	identity.MemberWork
	inspected chan struct{}
	release   chan struct{}
}

func (p pausedMemberInspection) Inspect(ctx context.Context, tx pgx.Tx, source ids.UUID, lock bool) (identity.MemberWorkInspection, error) {
	result, err := p.MemberWork.Inspect(ctx, tx, source, lock)
	close(p.inspected)
	select {
	case <-p.release:
	case <-ctx.Done():
		return result, ctx.Err()
	}
	return result, err
}

func TestAMemberRetirementPreviewDoesNotHoldMutationLocks(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Concurrent work", pipeline, stage, &e.Rep1)
	probe := pausedMemberInspection{MemberWork: memberWork{newHandoverSeam(newBulkEngine(e.DB(), nil))}, inspected: make(chan struct{}), release: make(chan struct{})}
	manager.service.WithMemberWork(probe)
	ctx, cancel := context.WithCancel(e.As(e.AdminUser, nil, integration.AdminWithSignals))
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true); result <- err }()
	<-probe.inspected
	err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for _, key := range []string{"margince:authz", "margince:admin-guard"} {
			var free bool
			if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1)::bigint)`, key).Scan(&free); err != nil {
				return err
			}
			if !free {
				return fmt.Errorf("preview holds %s", key)
			}
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM app_user WHERE id=$1 FOR UPDATE NOWAIT`, e.Rep1); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT id FROM deal WHERE id=$1 FOR UPDATE NOWAIT`, deal)
		return err
	})
	close(probe.release)
	if previewErr := <-result; previewErr != nil {
		t.Fatal(previewErr)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestAConcurrentTaskAssignmentCannotLeaveWorkOnARetiredMember(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	task := assignedTask(t, e, "Concurrent assignment", e.Rep2, false)
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	merged := make(chan error, 1)
	assigned := make(chan error, 1)
	go func() { <-start; merged <- manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token) }()
	go func() {
		<-start
		_, err := e.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](e.Rep1))})
		assigned <- err
	}()
	close(start)
	mergeErr, assignmentErr := <-merged, <-assigned
	if assignmentErr != nil && !memberWriteRaceRefusal(assignmentErr) {
		t.Fatalf("assignment = %v", assignmentErr)
	}
	if mergeErr != nil {
		if !errors.Is(mergeErr, apperrors.ErrVersionSkew) && !memberWriteRaceRefusal(mergeErr) {
			t.Fatalf("merge = %v", mergeErr)
		}
		preview, err = manager.Preview(ctx, e.Rep1, &e.Rep2, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE assignee_id=$1 AND kind='task' AND NOT is_done AND archived_at IS NULL`, e.Rep1); got != "0" {
		t.Fatalf("retired member owns %s open tasks", got)
	}
	if e.WsScalar(t, `SELECT (removed_at IS NOT NULL)::text FROM app_user WHERE id=$1`, e.Rep1) != "true" {
		t.Fatal("source was not retired")
	}
}

func memberWriteRaceRefusal(err error) bool {
	// Not found is the merge having won: a retired seat cannot hold work.
	// seatHoldingWork answers that as not found, so a caller cannot tell it
	// from a guessed id. TestATaskCannotBeAssignedToARetiredMember pins the
	// same refusal without a race.
	if errors.Is(err, apperrors.ErrNotFound) {
		return true
	}
	if _, ok := errors.AsType[*auth.AssigneeNotAllowedError](err); ok {
		return true
	}
	databaseError, ok := errors.AsType[*pgconn.PgError](err)
	return ok && (databaseError.Code == "40P01" || databaseError.Code == "40001" || databaseError.ConstraintName == "member_work_reference_retired")
}
