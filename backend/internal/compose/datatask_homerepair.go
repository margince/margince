// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const homeRepairTaskID = "core/1792501453_home_attention_repair"

const (
	repairRunID       = "run_id"
	repairSourceField = "source"
)

func homeRepairDataTask(pool *pgxpool.Pool, now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: homeRepairTaskID, Version: 1, Title: "Review automatic Home work", Manual: true, ReviewVersion: homeRepairVersion,
		Requires: []string{"core/1792501453"}, Review: validateHomeRepairTx,
		Steps: []datatask.Step{{
			Name: "reviewed_tasks", Limit: 50, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) { return 0, false, nil },
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return applyHomeRepairBatch(ctx, tx, pool, in, now())
			},
		}},
	}
}

func applyHomeRepairBatch(ctx context.Context, tx pgx.Tx, pool *pgxpool.Pool, in datatask.BatchIn, asOf time.Time) (datatask.BatchOut, error) {
	if in.Activation == nil {
		return datatask.BatchOut{}, apperrors.ErrPermissionDenied
	}
	human, ok := principal.HumanUserID(in.Activation.Operator)
	if !ok {
		return datatask.BatchOut{}, apperrors.ErrPermissionDenied
	}
	actor, err := (storedActor{authority: identity.NewService(pool)}).rebuild(ctx, migration.StoredActor{Type: principal.PrincipalHuman, UserID: human})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	ctx = principal.WithActor(ctx, actor)
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return datatask.BatchOut{}, err
	}
	var scope homeRepairScope
	if err := json.Unmarshal(in.Activation.Scope, &scope); err != nil {
		return datatask.BatchOut{}, err
	}
	offset := 0
	if len(in.Cursor) > 0 {
		if err := json.Unmarshal(in.Cursor, &offset); err != nil {
			return datatask.BatchOut{}, err
		}
	}
	if offset < 0 || offset > len(scope.Tasks) {
		return datatask.BatchOut{}, apperrors.ErrInvalidArgument
	}
	end := min(offset+in.Limit, len(scope.Tasks))
	store := activities.NewStore(InstallationDB(pool))
	activation := *in.Activation
	activation.AsOf = asOf
	changed := 0
	for _, expected := range scope.Tasks[offset:end] {
		wrote, err := applyHomeRepairItem(ctx, tx, store, expected, activation, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if wrote {
			changed++
		}
	}
	cursor, err := json.Marshal(end)
	return datatask.BatchOut{Cursor: cursor, Scanned: end - offset, Changed: changed, Done: end == len(scope.Tasks)}, err
}

func applyHomeRepairItem(ctx context.Context, tx pgx.Tx, store *activities.Store, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	if expected.Source == repairSourceVerdict {
		return applyHomeRepairVerdict(ctx, tx, expected, activation, runID)
	}
	if expected.Source == repairSourceSuggestion {
		return applyHomeRepairSuggestion(ctx, tx, expected, activation, runID)
	}
	if expected.Source == repairSourceApproval {
		return applyHomeRepairProposal(ctx, tx, expected, activation, runID)
	}
	if expected.Source == repairSourceSettlement {
		return applyHomeRepairSettlement(ctx, tx, expected, activation, runID)
	}
	if expected.Source != "" && expected.Source != repairSourceTask {
		return false, apperrors.ErrInvalidArgument
	}
	return applyHomeRepairTask(ctx, tx, store, expected, activation, runID)
}

func applyHomeRepairTask(ctx context.Context, tx pgx.Tx, store *activities.Store, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	task, err := activities.HomeRepairTaskTx(ctx, tx, expected.ID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, recordHomeRepairSkip(ctx, tx, expected, "source_or_task_unavailable", activation.RuleVersion, runID)
	}
	if err != nil {
		return false, err
	}
	decision, err := homeRepairTaskDecisionTx(ctx, tx, task, activation.AsOf, true)
	if err != nil {
		return false, err
	}
	if task.Version != expected.Version || decision.Action != expected.Action || decision.Reason != expected.Reason {
		return false, recordHomeRepairSkip(ctx, tx, expected, "changed_since_review", activation.RuleVersion, runID)
	}
	if decision.Action != repairArchiveReminder {
		return false, recordHomeRepairSkip(ctx, tx, expected, decision.Reason, activation.RuleVersion, runID)
	}
	err = pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
		return store.ArchiveActivityTx(ctx, savepoint, task.ID, &expected.Version, activities.OnlyPristineAutomaticTask(), activities.WithArchiveReason(decision.Reason, activation.RuleVersion))
	})
	if errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrRetentionHold) || errors.Is(err, apperrors.ErrNotFound) {
		return false, recordHomeRepairSkip(ctx, tx, expected, "write_refused_since_review", activation.RuleVersion, runID)
	}
	return err == nil, err
}

func recordHomeRepairSkip(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, reason, version string, runID ids.UUID) error {
	_, err := storekit.LogSystem(ctx, tx, "home_repair.skipped", map[string]any{repairRunID: runID, repairRecordID: expected.ID, repairSourceField: expected.Source, "reason": reason, "rule_version": version})
	return err
}
