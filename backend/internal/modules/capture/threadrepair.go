// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ThreadRepairActor prevents derived repairs from enqueueing themselves as new input.
const ThreadRepairActor = "system:thread_repair"

const (
	threadArgKey       = "key"
	threadArgComponent = "component"
	threadArgJob       = "job"
	threadArgZero      = "zero"
	threadArgAfter     = "after"
	threadArgSeat      = "seat"
	threadArgTopic     = "topic"
	threadArgSource    = "source"
	threadArgParent    = "parent"
	threadArgRoot      = "root"
	threadPhaseRepair  = "repair"
	threadPhaseTopics  = "topics"
	threadPhaseLabels  = "labels"
	threadPhaseRoots   = "roots"
	threadPhaseInherit = "inherit"
	threadPhaseApply   = "apply"
)

// ThreadRepairApplier keeps each domain write in its owning module.
type ThreadRepairApplier struct {
	Invalidate     func(context.Context, pgx.Tx, ids.ActivityID) (bool, error)
	Hold           func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error
	PublishSeat    func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error
	Notify         func(context.Context, pgx.Tx, ids.ActivityID, string) error
	AuthorizeEdge  func(context.Context, pgx.Tx, ids.ActivityID, ids.ActivityID) (bool, error)
	Activity       func(context.Context, pgx.Tx, ids.ActivityID, string, string) (bool, error)
	Delivery       func(context.Context, pgx.Tx, ids.UUID, ids.ActivityID, string, string) error
	Verdict        func(context.Context, pgx.Tx, string, string, ids.UUID) error
	Basis          func(context.Context, pgx.Tx, ids.UUID, string, string) error
	Representative func(context.Context, pgx.Tx, string, ids.UUID) error
}

type threadRepair struct {
	ID      ids.UUID
	Phase   string
	After   ids.UUID
	RunID   ids.UUID
	Resumed bool
}

type threadRepairNode struct {
	ID        ids.ActivityID
	Source    string
	ParentKey string
	Parent    *ids.ActivityID
	Version   int64
	State     threadBoundaryState
}

// QueueThreadRepairTx retains recovery work; a deferred newborn stays hidden until ready.
func QueueThreadRepairTx(ctx context.Context, tx pgx.Tx, key string, blocked ...bool) error {
	if key == "" {
		return nil
	}
	hide := len(blocked) > 0 && blocked[0]
	_, err := tx.Exec(ctx, `INSERT INTO capture_thread_pending(source_key,blocked) SELECT @key,@hide WHERE NOT EXISTS(SELECT 1 FROM capture_thread_barrier b JOIN capture_thread_repair r ON r.id=b.repair_id WHERE b.blocked_key=@key AND r.phase<>'published') ON CONFLICT(source_key) DO UPDATE SET blocked=capture_thread_pending.blocked OR EXCLUDED.blocked`, pgx.NamedArgs{threadArgKey: key, "hide": hide})
	return err
}

// ThreadRepairBatchTx commits at most limit persisted graph or owner operations.
// Publication changes one phase row; barrier and graph retirement remain bounded.
func ThreadRepairBatchTx(ctx context.Context, tx pgx.Tx, limit int, run ids.UUID, apply ThreadRepairApplier, component ...string) (int, int, bool, error) {
	if err := validateThreadRepair(limit, apply); err != nil {
		return 0, 0, false, err
	}
	if err := LockThreadMergesTx(ctx, tx); err != nil {
		return 0, 0, false, err
	}
	key := ""
	if len(component) > 0 {
		key = component[0]
	}
	unrelated, err := unrelatedThreadRepairTx(ctx, tx, key)
	if err != nil || unrelated {
		return 0, 0, false, err
	}
	scanned, changed := 0, 0
	for scanned < limit {
		invalidated, err := invalidateThreadContentUnitTx(ctx, tx, key, apply)
		if err != nil {
			return scanned, changed, false, err
		}
		if invalidated {
			scanned++
			continue
		}
		job, found, err := nextThreadRepairTx(ctx, tx, run, key)
		if err != nil {
			return scanned, changed, false, err
		}
		if !found {
			return scanned, changed, true, nil
		}
		unitCtx := principal.WithActor(principal.WithCorrelationID(ctx, job.RunID), principal.Principal{Type: principal.PrincipalSystem, ID: ThreadRepairActor})
		moved, err := isolatedThreadRepairUnitTx(unitCtx, tx, job, apply)
		if err != nil {
			return scanned, changed, false, err
		}
		scanned++
		if moved {
			changed++
		}
	}
	return scanned, changed, false, nil
}

// LockThreadMergesTx preserves capture-before-row lock ordering for repair callers.
func LockThreadMergesTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, mergeLockStatement)
	return err
}

func nextThreadRepairTx(ctx context.Context, tx pgx.Tx, run ids.UUID, component string) (threadRepair, bool, error) {
	var job threadRepair
	if _, err := tx.Exec(ctx, `UPDATE capture_thread_repair r SET state='backoff',retry_after=statement_timestamp() WHERE r.phase<>'published' AND r.state='ready' AND `+threadRepairInvalidationSQL("r.id")); err != nil {
		return job, false, err
	}
	err := tx.QueryRow(ctx, `SELECT id,phase,after_activity,run_id,was_blocked OR created_xid<>pg_current_xact_id() FROM capture_thread_repair WHERE phase <> 'published' AND state='ready' LIMIT 1`).Scan(&job.ID, &job.Phase, &job.After, &job.RunID, &job.Resumed)
	if err == nil {
		return job, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return job, false, err
	}
	var published ids.UUID
	var publishedRun ids.UUID
	var resumed bool
	err = tx.QueryRow(ctx, `SELECT id,run_id,was_blocked OR created_xid<>pg_current_xact_id() FROM capture_thread_repair WHERE phase='published' AND state='ready' AND NOT (`+threadRepairInvalidationSQL("capture_thread_repair.id")+`) AND (@component='' OR EXISTS(SELECT 1 FROM capture_thread_barrier b WHERE b.repair_id=capture_thread_repair.id AND b.blocked_key=@component)) ORDER BY id LIMIT 1`, pgx.NamedArgs{threadArgComponent: component}).Scan(&published, &publishedRun, &resumed)
	if err == nil {
		return threadRepair{ID: published, Phase: "cleanup", RunID: publishedRun, Resumed: resumed}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return job, false, err
	}
	resumedJob, err := resumeThreadRepairTx(ctx, tx, component)
	if err != nil {
		return job, false, err
	}
	if resumedJob {
		return nextThreadRepairTx(ctx, tx, run, component)
	}
	var key string
	var blocked bool
	err = tx.QueryRow(ctx, `DELETE FROM capture_thread_pending WHERE source_key=(SELECT p.source_key FROM capture_thread_pending p WHERE (@component='' OR p.source_key=@component) AND NOT EXISTS(SELECT 1 FROM capture_thread_barrier b JOIN capture_thread_repair r ON r.id=b.repair_id WHERE b.blocked_key=p.source_key AND r.phase<>'published') ORDER BY p.queue_order LIMIT 1) RETURNING source_key,blocked`, pgx.NamedArgs{threadArgComponent: component}).Scan(&key, &blocked)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, false, nil
	}
	if err != nil {
		return job, false, err
	}
	job.ID = ids.NewV7()
	job.RunID = run
	job.Resumed = blocked
	job.Phase = "load"
	_, err = tx.Exec(ctx, `INSERT INTO capture_thread_repair(id,run_id,was_blocked) VALUES(@id,@run,@blocked)`, pgx.NamedArgs{"id": job.ID, "run": run, "blocked": blocked})
	if err != nil {
		return job, false, err
	}
	return job, true, addThreadBarrierTx(ctx, tx, job.ID, key)
}

func addThreadBarrierTx(ctx context.Context, tx pgx.Tx, job ids.UUID, key string) error {
	if key == "" {
		return nil
	}
	var competing bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_barrier b JOIN capture_thread_repair r ON r.id=b.repair_id WHERE b.blocked_key=@key AND b.repair_id<>@job AND r.phase<>'published')`, pgx.NamedArgs{threadArgJob: job, threadArgKey: key}).Scan(&competing); err != nil {
		return err
	}
	if competing {
		return errors.New("capture: repair component is waiting for another repair")
	}
	tag, err := tx.Exec(ctx, `INSERT INTO capture_thread_barrier(repair_id,blocked_key) VALUES(@job,@key) ON CONFLICT DO NOTHING`, pgx.NamedArgs{threadArgJob: job, threadArgKey: key})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET was_blocked=true WHERE id=@job AND EXISTS(SELECT 1 FROM capture_thread_pending WHERE source_key=@key AND blocked)`, pgx.NamedArgs{threadArgJob: job, threadArgKey: key}); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM capture_thread_pending WHERE source_key=@key`, pgx.NamedArgs{threadArgKey: key})
	return err
}

func threadRepairPhaseTx(ctx context.Context, tx pgx.Tx, job ids.UUID, phase string) error {
	if phase == "published" {
		invalid, err := threadRepairInvalidatedTx(ctx, tx, job)
		if err != nil || invalid {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET phase=@phase,after_activity=@zero WHERE id=@job`, pgx.NamedArgs{threadArgJob: job, "phase": phase, threadArgZero: ids.Nil})
	return err
}

func threadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, apply ThreadRepairApplier) (bool, error) {
	invalid, err := threadRepairInvalidatedTx(ctx, tx, job.ID)
	if err != nil || invalid {
		return false, err
	}
	switch job.Phase {
	case "load":
		return false, loadThreadRepairUnitTx(ctx, tx, job)
	case "refresh":
		return false, refreshThreadRepairUnitTx(ctx, tx, job)
	case "cleanup":
		return false, cleanThreadRepairUnitTx(ctx, tx, job, apply)
	case "discover":
		return false, discoverThreadRepairUnitTx(ctx, tx, job, apply)
	case threadPhaseTopics, threadPhaseLabels, threadPhaseRoots:
		return false, advanceThreadRepairUnitTx(ctx, tx, job)
	case "targets":
		return false, discoverThreadTargetUnitTx(ctx, tx, job)
	case threadPhaseInherit, threadPhaseApply, threadPhaseRepair:
		return applyThreadRepairUnitTx(ctx, tx, job, apply)
	default:
		return false, errors.New("capture: unknown thread repair phase")
	}
}

func validateThreadRepair(limit int, apply ThreadRepairApplier) error {
	if apply.Invalidate == nil || apply.AuthorizeEdge == nil || apply.Activity == nil || apply.Delivery == nil || apply.Verdict == nil || apply.Basis == nil || apply.Representative == nil {
		return errors.New("capture: thread repair collaborators are incomplete")
	}
	if limit < 1 {
		return errors.New("capture: repair batch limit must be positive")
	}
	return nil
}

func unrelatedThreadRepairTx(ctx context.Context, tx pgx.Tx, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var unrelated bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_repair r WHERE r.phase<>'published' AND r.state='ready' AND NOT EXISTS(SELECT 1 FROM capture_thread_barrier b WHERE b.repair_id=r.id AND b.blocked_key=@key))`, pgx.NamedArgs{threadArgKey: key}).Scan(&unrelated)
	return unrelated, err
}
