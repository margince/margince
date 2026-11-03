// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const ruleVersionEvidenceKey = "rule_version"

// AutomaticTaskRepair carries source references without copying their content.
type AutomaticTaskRepair struct {
	SourceID        *ids.UUID
	Supported       bool
	Current         bool
	NormalizeSource bool
	ClearDue        bool
}

// AutomaticTaskRepairTx gives preview and apply the same source and date policy.
func AutomaticTaskRepairTx(ctx context.Context, tx pgx.Tx, task HomeRepairTask, asOf time.Time, forDecision bool) (AutomaticTaskRepair, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return AutomaticTaskRepair{}, err
	}
	actual, err := HomeRepairTaskTx(ctx, tx, task.ID.UUID)
	if err != nil {
		return AutomaticTaskRepair{}, err
	}
	if actual.Version != task.Version {
		return AutomaticTaskRepair{}, apperrors.ErrVersionSkew
	}
	task = actual
	supported, requiresSource := storekit.TaskRepairPolicy(task.SourceSystem)
	if !supported {
		return AutomaticTaskRepair{}, nil
	}
	if forDecision {
		if _, err := storekit.LockRow(ctx, tx, "activity", task.ID.UUID, storekit.LiveOnly); err != nil {
			return AutomaticTaskRepair{}, err
		}
	}
	basis, err := automaticTaskRepairBasis(ctx, tx, task, asOf)
	if err != nil || !requiresSource {
		return basis, err
	}
	if basis.SourceID == nil {
		return AutomaticTaskRepair{}, apperrors.ErrNotFound
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, *basis.SourceID); err != nil {
		return AutomaticTaskRepair{}, err
	}
	if forDecision {
		if _, err := storekit.LockRow(ctx, tx, "activity", *basis.SourceID, storekit.IncludeArchived); err != nil {
			return AutomaticTaskRepair{}, err
		}
		if err := auth.EnsureActivityContentVisible(ctx, tx, *basis.SourceID); err != nil {
			return AutomaticTaskRepair{}, err
		}
		return automaticTaskRepairBasis(ctx, tx, task, asOf)
	}
	return basis, nil
}

func automaticTaskRepairBasis(ctx context.Context, tx pgx.Tx, task HomeRepairTask, asOf time.Time) (AutomaticTaskRepair, error) {
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	id, version := arg(task.ID), arg(task.Version)
	current := storekit.CurrentTaskClause("a", asOf, "'[]'::jsonb", arg)
	query := storekit.SQLf(`SELECT %s, a.source_activity_id IS NULL,
 a.due_at IS NOT NULL AND (%s) IS NULL, %s FROM activity a WHERE a.id=$%d AND a.version=$%d`,
		provenance.TaskSourceActivitySQL("a"), storekit.TaskDueSQL("a"), current, id, version)
	basis := AutomaticTaskRepair{Supported: true}
	err := tx.QueryRow(ctx, query, args...).Scan(&basis.SourceID, &basis.NormalizeSource, &basis.ClearDue, &basis.Current)
	if errors.Is(err, pgx.ErrNoRows) {
		return basis, apperrors.ErrVersionSkew
	}
	basis.NormalizeSource = basis.NormalizeSource && basis.SourceID != nil
	return basis, err
}

// RepairAutomaticTaskTx records the operator without turning a correction into adoption.
func (s *Store) RepairAutomaticTaskTx(ctx context.Context, tx pgx.Tx, task HomeRepairTask, asOf time.Time, ruleVersion string) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	input := UpdateActivityInput{IfVersion: &task.Version}
	if _, _, err := admitActivityPatch(ctx, tx, task.ID, &input); err != nil {
		return err
	}
	if err := refuseIfTouched(ctx, tx, task.ID, archiveOptions{pristineTask: true}); err != nil {
		return err
	}
	basis, err := AutomaticTaskRepairTx(ctx, tx, task, asOf, true)
	if err != nil {
		return err
	}
	if !basis.Supported || !basis.Current || (!basis.NormalizeSource && !basis.ClearDue) {
		return apperrors.ErrConflict
	}
	before, after := automaticRepairImages(task, basis)
	args := pgx.NamedArgs{
		"id": task.ID, "version": task.Version, "normalize": basis.NormalizeSource,
		ledgerSourceKey: basis.SourceID, "clear_due": basis.ClearDue,
	}
	var nextVersion int64
	query := `UPDATE activity SET source_activity_id=CASE WHEN @normalize THEN @source::uuid ELSE source_activity_id END,
 due_at=CASE WHEN @clear_due THEN NULL ELSE due_at END
 WHERE id=@id AND version=@version AND archived_at IS NULL RETURNING version`
	if err := tx.QueryRow(ctx, query, args).Scan(&nextVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrVersionSkew
		}
		return err
	}
	evidence := map[string]any{
		"automatic_task_repair": "1", ruleVersionEvidenceKey: ruleVersion,
		"before_version": task.Version, "after_version": nextVersion,
	}
	audit, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", task.ID.UUID, before, after, evidence)
	if err != nil {
		return err
	}
	event := crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{AutomaticTaskRepaired: new(true)}}
	if err := storekit.EmitEvent(ctx, tx, audit, task.ID.UUID, event); err != nil {
		return fmt.Errorf("recording automatic task correction: %w", err)
	}
	return nil
}

func automaticRepairImages(task HomeRepairTask, basis AutomaticTaskRepair) (map[string]any, map[string]any) {
	before, after := map[string]any{}, map[string]any{}
	if basis.NormalizeSource {
		before["source_activity_id"], after["source_activity_id"] = task.SourceID, basis.SourceID
	}
	if basis.ClearDue {
		before["due_at"], after["due_at"] = task.DueAt, nil
	}
	return before, after
}
