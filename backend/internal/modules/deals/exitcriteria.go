// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A stage's exit criteria: the configuration half of stage intelligence.
// A criterion says what this stage REQUIRES; whether a given deal has met
// it is evidence, recorded elsewhere against the criterion's id. Keeping
// the two apart is what lets an admin reword a criterion without rewriting
// what was already observed under it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// criterionEntity is the audit and event entity name for a criterion row.
const criterionEntity = "stage_exit_criterion"

const (
	codeTerminalStageNoCriteria = "terminal_stage_has_no_exit_criteria"
	codeCriterionKeyTaken       = "criterion_key_taken"
)

// criterionColumns is the SELECT list both criterion reads build their
// statement from. readCriteria and readCriterion concatenate this constant
// rather than typing columns, so scanCriterion's fixed Scan order stays
// correct for both: a column added here without a matching Scan dest fails
// to compile, which is the check that keeps them agreeing.
const criterionColumns = `id, stage_id, key, label, kind, required, hint,
	"position", version, created_at, updated_at, archived_at`

// CreateCriterionInput is what a caller says to add a criterion to a stage.
// Required defaults to true when absent: a criterion nobody has to meet is
// the exception, so it is the answer a caller states rather than the one they
// fall into. A criterion always appends — see nextCriterionPosition.
type CreateCriterionInput struct {
	StageID  ids.StageID
	Key      string
	Label    string
	Kind     string
	Required *bool
	Hint     *string
}

// UpdateCriterionInput names only the fields an edit moves; a nil pointer
// leaves its field standing. Key is absent by design — evidence matches a
// criterion by key across an edit, so renaming one would orphan every claim
// already recorded under it. Position is absent too: moving one row is half a
// reorder, because the row it displaces keeps its own slot.
//
// SetHint is what separates "leave the hint alone" from "clear it", because
// both arrive as a nil Hint.
type UpdateCriterionInput struct {
	Label     *string
	Kind      *string
	Required  *bool
	Hint      *string
	SetHint   bool
	IfVersion *int64
}

// ListStageExitCriteria answers a stage's criteria in position order.
//
// Reading pipeline shape needs only a seat, like ListStages: a criterion is
// configuration every rep sees on the board, not a record with an owner.
func (s *Store) ListStageExitCriteria(
	ctx context.Context, stageID ids.StageID, archived storekit.ArchivedFilter,
) ([]crmcontracts.StageExitCriterion, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.StageExitCriterion
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The stage must EXIST, but it need not be live. Archiving a stage
		// keeps its criteria on disk so evidence recorded against them stays
		// readable, and a read that refused an archived parent would hide
		// exactly the history the archive was preserving.
		if err := requireStage(ctx, tx, stageID, archived); err != nil {
			return err
		}
		var err error
		out, err = readCriteria(ctx, tx, stageID, archived)
		return err
	})
	return out, err
}

// CreateStageExitCriterion adds a criterion to a live, non-terminal stage.
func (s *Store) CreateStageExitCriterion(
	ctx context.Context, in CreateCriterionInput,
) (crmcontracts.StageExitCriterion, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.StageExitCriterion{}, err
	}
	kind, err := ParseCriterionKind(in.Kind)
	if err != nil {
		return crmcontracts.StageExitCriterion{}, err
	}
	if err := validCriterionKey(in.Key); err != nil {
		return crmcontracts.StageExitCriterion{}, err
	}
	if err := validCriterionLabel(in.Label); err != nil {
		return crmcontracts.StageExitCriterion{}, err
	}
	var out crmcontracts.StageExitCriterion
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		// The stage row serializes every reshaping of its criteria list, so
		// two concurrent adds cannot both compute the same tail position.
		if _, err := lockCriteriaList(ctx, tx, in.StageID); err != nil {
			return err
		}
		if err := refuseCriterionOnTerminalStage(ctx, tx, in.StageID); err != nil {
			return err
		}
		if err := refuseTakenCriterionKey(ctx, tx, in.StageID, in.Key); err != nil {
			return err
		}
		position, err := nextCriterionPosition(ctx, tx, in.StageID)
		if err != nil {
			return err
		}
		id, err := insertCriterion(ctx, tx, in, kind, position)
		if err != nil {
			return err
		}
		if out, err = readCriterion(ctx, tx, id, storekit.IncludeArchived); err != nil {
			return err
		}
		return announceCriterion(ctx, tx, in.StageID, id, criterionAfter(out))
	})
	return out, err
}

// insertCriterion writes the row and answers its id.
func insertCriterion(
	ctx context.Context, tx pgx.Tx, in CreateCriterionInput, kind CriterionKind, position int,
) (ids.ExitCriterionID, error) {
	required := true
	if in.Required != nil {
		required = *in.Required
	}
	var id ids.ExitCriterionID
	err := tx.QueryRow(ctx, `
		INSERT INTO stage_exit_criterion (stage_id, key, label, kind, required, hint, "position")
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		in.StageID, in.Key, in.Label, string(kind), required, in.Hint, position).Scan(&id)
	if err != nil {
		return id, fmt.Errorf("insert exit criterion: %w", err)
	}
	return id, nil
}

// UpdateStageExitCriterion edits a criterion. The key is not editable —
// evidence cites a criterion by key across an edit, so a rename would
// silently orphan every claim already recorded under the old one.
func (s *Store) UpdateStageExitCriterion(
	ctx context.Context, stageID ids.StageID, id ids.ExitCriterionID, in UpdateCriterionInput,
) (crmcontracts.StageExitCriterion, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.StageExitCriterion{}, err
	}
	if in.Kind != nil {
		if _, err := ParseCriterionKind(*in.Kind); err != nil {
			return crmcontracts.StageExitCriterion{}, err
		}
	}
	if in.Label != nil {
		if err := validCriterionLabel(*in.Label); err != nil {
			return crmcontracts.StageExitCriterion{}, err
		}
	}
	var out crmcontracts.StageExitCriterion
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := lockCriteriaList(ctx, tx, stageID); err != nil {
			return err
		}
		lock, err := storekit.LockRow(ctx, tx, criterionEntity, id.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		current, err := readCriterion(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if current.StageId != openapiUUID(stageID.UUID) {
			return apperrors.ErrNotFound
		}
		if in.IfVersion != nil && *in.IfVersion != criterionVersion(current) {
			return apperrors.ErrVersionSkew
		}
		if patch := criterionUpdatePatch(current, in); !patch.Empty() {
			if err := writeCriterionUpdate(ctx, tx, lock, stageID, id, patch); err != nil {
				return err
			}
		}
		out, err = readCriterion(ctx, tx, id, storekit.LiveOnly)
		return err
	})
	return out, err
}

// writeCriterionUpdate commits the patch, its audit row and the event.
func writeCriterionUpdate(ctx context.Context, tx pgx.Tx, lock storekit.RowLock,
	stageID ids.StageID, id ids.ExitCriterionID, patch *storekit.Patch,
) error {
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("update exit criterion: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", criterionEntity, id.UUID,
		patch.Before(), patch.After())
	if err != nil {
		return fmt.Errorf("audit exit criterion update: %w", err)
	}
	return emitStageCriteriaChanged(ctx, tx, auditID, stageID)
}

// ArchiveStageExitCriterion removes a criterion from its stage.
//
// Archive, never delete: evidence references the criterion it settled, and a
// reader opening an older deal must still see what the stage asked for then.
func (s *Store) ArchiveStageExitCriterion(
	ctx context.Context, stageID ids.StageID, id ids.ExitCriterionID, ifVersion *int64,
) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := lockCriteriaList(ctx, tx, stageID); err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, criterionEntity, id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		current, err := readCriterion(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if current.StageId != openapiUUID(stageID.UUID) {
			return apperrors.ErrNotFound
		}
		if ifVersion != nil && *ifVersion != criterionVersion(current) {
			return apperrors.ErrVersionSkew
		}
		if _, err := tx.Exec(ctx, `
			UPDATE stage_exit_criterion
			   SET archived_at = now()
			 WHERE id = $1 AND archived_at IS NULL`, id); err != nil {
			return fmt.Errorf("archive exit criterion: %w", err)
		}
		// The survivors close the gap, so position stays contiguous and the
		// editor never draws a hole where the removed row was.
		tag, err := tx.Exec(ctx, `
			UPDATE stage_exit_criterion
			   SET "position" = "position" - 1
			 WHERE stage_id = $1 AND archived_at IS NULL AND "position" > $2`,
			stageID, current.Position)
		if err != nil {
			return fmt.Errorf("close the gap the archived criterion left: %w", err)
		}
		moved := tag.RowsAffected()
		// The renumbering is part of what this archive DID, so the after-image
		// says how many rows moved up behind it. Without that the trail shows
		// one criterion archived while several others quietly changed position
		// and version, and nothing accounts for the difference.
		auditID, err := storekit.Audit(ctx, tx, "archive", criterionEntity, id.UUID,
			criterionAfter(current),
			map[string]any{"archived": true, "siblings_renumbered": moved})
		if err != nil {
			return fmt.Errorf("audit exit criterion archive: %w", err)
		}
		return emitStageCriteriaChanged(ctx, tx, auditID, stageID)
	})
}

// announceCriterion writes the create's audit row and the stage event.
//
// The verb is a literal rather than a parameter: a runtime-built verb is one
// the before-image gate cannot judge, and this has exactly one caller. Archive
// spells its own "archive" beside the before-image it can supply.
func announceCriterion(ctx context.Context, tx pgx.Tx,
	stageID ids.StageID, id ids.ExitCriterionID, after map[string]any,
) error {
	auditID, err := storekit.AuditEvent(ctx, tx, "create", criterionEntity, id.UUID, after)
	if err != nil {
		return fmt.Errorf("audit exit criterion create: %w", err)
	}
	return emitStageCriteriaChanged(ctx, tx, auditID, stageID)
}

// emitStageCriteriaChanged publishes the one fact a criterion write makes:
// this stage's exit criteria changed. It rides stage.updated because a
// criterion is part of the stage's shape, and subscribers already watch that
// for every other reshaping of a stage.
//
// The payload names no criterion field. A subscriber that needs the list
// re-reads it; putting the criteria inline would publish configuration to
// every consumer of a stage rename.
func emitStageCriteriaChanged(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID, stageID ids.StageID,
) error {
	pipelineID, err := pipelineOfStage(ctx, tx, stageID)
	if err != nil {
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, stageID.UUID,
		crmcontracts.PublicEventStageUpdated{
			PipelineId: openapiUUID(pipelineID.UUID),
		}); err != nil {
		return fmt.Errorf("emit stage.updated for its criteria: %w", err)
	}
	return nil
}

// pipelineOfStage answers the pipeline a live stage belongs to.
func pipelineOfStage(ctx context.Context, tx pgx.Tx, stageID ids.StageID) (ids.PipelineID, error) {
	var pipelineID ids.PipelineID
	err := tx.QueryRow(ctx,
		`SELECT pipeline_id FROM stage WHERE id = $1 AND archived_at IS NULL`, stageID).Scan(&pipelineID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pipelineID, apperrors.ErrNotFound
	}
	if err != nil {
		return pipelineID, fmt.Errorf("read the stage's pipeline: %w", err)
	}
	return pipelineID, nil
}
