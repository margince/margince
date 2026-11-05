// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedLegacyCorrectionRechecksAccessAfterWaitingForItsSource(t *testing.T) {
	f := seedLegacyRepair(t, false)
	reader := f.e.As(f.e.Rep1, nil, integration.AdminPerms)
	var task activities.HomeRepairTask
	if err := f.e.DB().Tx(reader, func(tx pgx.Tx) error {
		var err error
		task, err = activities.HomeRepairTaskTx(reader, tx, f.task.UUID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ready, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	written := make(chan error, 1)
	go func() {
		err := database.WithWorkspaceSnapshot(f.e.Admin(), f.e.Pool, func(ctx context.Context) error {
			if _, err := f.e.Activities.SetAudience(ctx, ids.From[ids.ActivityKind](f.source), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.Rep2}}}); err != nil {
				return err
			}
			close(ready)
			<-release
			return nil
		})
		written <- err
	}()
	select {
	case <-ready:
	case err := <-written:
		t.Fatalf("prepare source audience change: %v", err)
	}
	repaired, done := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		repaired <- f.e.DB().Tx(reader, func(tx pgx.Tx) error {
			return f.e.Activities.RepairAutomaticTaskTx(reader, tx, task, f.now, homeRepairVersion)
		})
	}()
	waitForLockWaiter(t, done, "'tuple', 'transactionid'", "the correction did not wait for its source")
	close(release)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if err := <-repaired; !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unreadable source gained authority after the lock: %v", err)
	}
	if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND version=@version AND source_activity_id IS NULL AND due_at IS NOT NULL", pgx.NamedArgs{"id": f.task, "version": task.Version}) != 1 {
		t.Fatal("refused correction changed the task")
	}
}

func TestReviewedLegacyWritesWaitForAConcurrentPinBeforeDecidingPristineness(t *testing.T) {
	for _, old := range []bool{false, true} {
		t.Run(map[bool]string{false: "source correction", true: "archive"}[old], func(t *testing.T) {
			f := seedLegacyRepair(t, old)
			review, item := f.preview(t)
			owner := f.e.As(f.e.Rep2, nil, integration.AdminPerms)
			ready, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			pinned := make(chan error, 1)
			go func() {
				pinned <- database.WithWorkspaceSnapshot(owner, f.e.Pool, func(ctx context.Context) error {
					if err := f.e.Activities.PinWorklistRow(ctx, activities.WorklistRowRef{Source: "task", RowID: f.task.String()}); err != nil {
						return err
					}
					close(ready)
					<-release
					return nil
				})
			}()
			select {
			case <-ready:
			case err := <-pinned:
				t.Fatalf("prepare uncommitted pin: %v", err)
			}
			done := make(chan struct{})
			type correction struct {
				changed bool
				err     error
			}
			repaired := make(chan correction, 1)
			go func() {
				defer close(done)
				changed := false
				err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
					var err error
					changed, err = applyHomeRepairTask(f.e.Admin(), tx, f.e.Activities, item, datatask.Activation{ReviewRequest: review}, ids.NewV7())
					return err
				})
				repaired <- correction{changed, err}
			}()
			waitForLockWaiter(t, done, "'advisory'", "the pristine write ignored a pin in progress")
			close(release)
			if err := <-pinned; err != nil {
				t.Fatal(err)
			}
			result := <-repaired
			if result.err != nil || result.changed {
				t.Fatalf("concurrent adoption was not preserved: %+v", result)
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id IS NULL AND due_at IS NOT NULL AND archived_at IS NULL AND NOT is_done", pgx.NamedArgs{"id": f.task}) != 1 {
				t.Fatal("the pinned task was changed")
			}
		})
	}
}
