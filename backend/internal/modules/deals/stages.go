// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Pipeline/stage configuration beyond create (B-EP02): bounded config
// mutations, each a first-class fact per events.md §5.3b — renames and
// probability changes ride stage.updated, reorders ride ONE
// pipeline.updated with the position delta (never N stage.updated).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

type UpdatePipelineInput struct {
	Name      *string
	IsDefault *bool
	Position  *int
	IfVersion *int64
}

func (s *Store) UpdatePipeline(ctx context.Context, id ids.PipelineID, in UpdatePipelineInput) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The row lock makes the read below and the update one race-free unit.
		lock, err := storekit.LockRow(ctx, tx, "pipeline", id.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		current, err := readPipelineConfig(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.IfVersion != nil && *in.IfVersion != current.version {
			return apperrors.ErrVersionSkew
		}
		// An update naming no field changes nothing, and an audit row for it
		// would record a transition that never happened.
		if patch := pipelineUpdatePatch(current, in); !patch.Empty() {
			promoting := in.IsDefault != nil && *in.IsDefault
			if err := writePipelineUpdate(ctx, tx, lock, id, patch, promoting); err != nil {
				return err
			}
		}
		if out, err = readPipeline(ctx, tx, id); err != nil {
			return fmt.Errorf("read updated pipeline: %w", err)
		}
		return nil
	})
	return out, err
}

// writePipelineUpdate commits the patch, its audit row and the one
// pipeline.updated event as a unit. The event's changed_fields is the patch's
// after-image, so the trail and the wire body report the same touched fields,
// and the before-image beside it says what each of them held first.
func writePipelineUpdate(ctx context.Context, tx pgx.Tx, lock storekit.RowLock,
	id ids.PipelineID, patch *storekit.Patch, promoting bool,
) error {
	// Exactly one default pipeline: promoting this one demotes the incumbent
	// in the same transaction.
	if promoting {
		if _, err := tx.Exec(ctx,
			`UPDATE pipeline SET is_default = false WHERE is_default AND id <> $1`, id); err != nil {
			return fmt.Errorf("demote incumbent default pipeline: %w", err)
		}
	}
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("update pipeline: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "pipeline", id.UUID, patch.Before(), patch.After())
	if err != nil {
		return fmt.Errorf("audit pipeline update: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventPipelineUpdated{
		ChangedFields: patch.After(),
	}); err != nil {
		return fmt.Errorf("emit pipeline.updated: %w", err)
	}
	return nil
}

type CreateStageInput struct {
	PipelineID     ids.PipelineID
	Name           string
	Position       int
	Semantic       string
	WinProbability *int
}

func (s *Store) CreateStage(ctx context.Context, in CreateStageInput) (crmcontracts.Stage, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.Stage{}, err
	}
	if in.Semantic == "" {
		in.Semantic = string(SemanticOpen)
	}
	if _, err := ParseStageSemantic(in.Semantic); err != nil {
		return crmcontracts.Stage{}, err
	}
	// The terminal-probability rule (won=100, lost=0) is a DDL CHECK;
	// filling the canonical value here turns an omitted probability into
	// the right one instead of a 500.
	probability := 0
	if in.WinProbability != nil {
		probability = *in.WinProbability
	} else if StageSemantic(in.Semantic) == SemanticWon {
		probability = 100
	}
	var out crmcontracts.Stage
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pipeline WHERE id = $1 AND archived_at IS NULL)`,
			in.PipelineID).Scan(&exists); err != nil {
			return fmt.Errorf("resolve pipeline: %w", err)
		}
		if !exists {
			return apperrors.ErrNotFound
		}
		var stageID ids.StageID
		err := tx.QueryRow(ctx, `
			INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id`,
			in.PipelineID, in.Name, in.Position, in.Semantic, probability).Scan(&stageID)
		if err != nil {
			if storekit.IsUniqueViolation(err) {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("insert stage: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "stage", stageID.UUID, nil, map[string]any{
			"pipeline_id": in.PipelineID, "name": in.Name, stageSemanticField: in.Semantic,
		})
		if err != nil {
			return fmt.Errorf("audit stage create: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, stageID.UUID, stageCreatedPayload(in.PipelineID, in.Name, in.Position, in.Semantic, probability)); err != nil {
			return fmt.Errorf("emit stage.created: %w", err)
		}
		if out, err = readStage(ctx, tx, stageID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read created stage: %w", err)
		}
		return nil
	})
	return out, err
}

// stageCreatedPayload builds the stage.created wire payload from
// CreateStage's resolved inputs — the ONE place that maps the local
// values onto the published schema, so a future field rename shows up
// here rather than at an independently-drifting map literal.
func stageCreatedPayload(pipelineID ids.PipelineID, name string, position int, semantic string, winProbability int) crmcontracts.PublicEventStageCreated {
	return crmcontracts.PublicEventStageCreated{
		PipelineId:     openapi_types.UUID(pipelineID.UUID),
		Name:           name,
		Position:       position,
		Semantic:       semantic,
		WinProbability: winProbability,
	}
}

func (s *Store) GetStage(ctx context.Context, id ids.StageID) (crmcontracts.Stage, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return crmcontracts.Stage{}, err
	}
	var out crmcontracts.Stage
	err := s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readStage(ctx, tx, id, storekit.IncludeArchived)
		return err
	})
	return out, err
}

func (s *Store) ListStages(ctx context.Context, pipelineID *ids.PipelineID, archived storekit.ArchivedFilter) ([]crmcontracts.Stage, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.Stage
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		where := predicateAlways
		if pipelineID != nil {
			where = storekit.SQLf("pipeline_id = $%d", arg(*pipelineID))
		}
		if archived == storekit.LiveOnly {
			where += " AND archived_at IS NULL"
		}
		rows, err := tx.Query(ctx, storekit.SQLf(
			`SELECT id FROM stage WHERE %s ORDER BY pipeline_id, position`, where,
		), args...)
		if err != nil {
			return err
		}
		var stageIDs []ids.StageID
		for rows.Next() {
			var id ids.StageID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			stageIDs = append(stageIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range stageIDs {
			stage, err := readStage(ctx, tx, id, storekit.IncludeArchived)
			if err != nil {
				return err
			}
			out = append(out, stage)
		}
		return nil
	})
	return out, err
}

type UpdateStageInput struct {
	Name           *string
	Position       *int
	Semantic       *string
	WinProbability *int
	IfVersion      *int64
}

func (s *Store) UpdateStage(ctx context.Context, id ids.StageID, in UpdateStageInput) (crmcontracts.Stage, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.Stage{}, err
	}
	var out crmcontracts.Stage
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The pipeline row first, then the stage's own: a reorder and a
		// removal both reshape this list, and taking the same row first
		// on both paths is what keeps them queueing rather than
		// deadlocking (lockStageConfig).
		pipelineID, err := lockStageConfig(ctx, tx, id)
		if err != nil {
			return err
		}
		// The row lock makes the read below and the update one race-free unit.
		lock, err := storekit.LockRow(ctx, tx, "stage", id.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		current, err := readStageConfig(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.IfVersion != nil && *in.IfVersion != current.version {
			return apperrors.ErrVersionSkew
		}
		// An update naming no field changes nothing, and an audit row for it
		// would record a transition that never happened.
		if patch := stageUpdatePatch(current, in); !patch.Empty() {
			if err := writeStageUpdate(ctx, tx, lock, id, pipelineID, patch, in); err != nil {
				return err
			}
		}
		if out, err = readStage(ctx, tx, id, storekit.IncludeArchived); err != nil {
			return fmt.Errorf("read updated stage: %w", err)
		}
		return nil
	})
	return out, err
}

// writeStageUpdate commits the patch, its audit row and the events this save
// earned. The audit images come from the patch, so `before` holds what each
// touched column really held and neither image mentions a column left alone.
func writeStageUpdate(ctx context.Context, tx pgx.Tx, lock storekit.RowLock,
	id ids.StageID, pipelineID ids.PipelineID, patch *storekit.Patch, in UpdateStageInput,
) error {
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("update stage: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "stage", id.UUID, patch.Before(), patch.After())
	if err != nil {
		return fmt.Errorf("audit stage update: %w", err)
	}
	// A reorder is a pipeline-level fact (pipeline.updated with the
	// position delta); a name/semantic/probability edit is a stage-level
	// fact (stage.updated). A single settings save can carry BOTH (the
	// UI sends position alongside the edited fields), so emit each fact
	// the update actually touched — they are NOT mutually exclusive.
	// Treating them as exclusive dropped stage.updated whenever a position
	// rode along, so a name/semantic change silently never reached
	// subscribers.
	if in.Position != nil {
		if err := storekit.EmitEvent(ctx, tx, auditID, pipelineID.UUID, crmcontracts.PublicEventPipelineUpdated{
			ChangedFields: map[string]any{"stage_positions": map[string]any{id.String(): *in.Position}},
		}); err != nil {
			return fmt.Errorf("emit pipeline reorder: %w", err)
		}
	}
	if in.Name != nil || in.Semantic != nil || in.WinProbability != nil {
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, stageUpdatedPayload(pipelineID, in)); err != nil {
			return fmt.Errorf("emit stage update: %w", err)
		}
	}
	return nil
}

// stageUpdatedPayload builds the stage.updated wire payload from
// UpdateStage's inputs — the ONE place that maps the local values onto
// the published schema. It carries only the fields this update actually
// touched (BOUNDED, not open — position never appears here: a position
// change publishes a pipeline.updated instead, per the caller's branch),
// so an untouched field stays a nil pointer and is omitted from the wire
// body rather than marshaled as null.
//
// A terminal semantic forces the committed win_probability (won → 100,
// lost → 0), so the payload MUST reflect that committed value, not the
// caller's input — otherwise a subscriber would see a win_probability that
// never hit the row. It reads that value from committedWinProbability, the
// same call the patch binds, rather than deriving it a second time.
func stageUpdatedPayload(pipelineID ids.PipelineID, in UpdateStageInput) crmcontracts.PublicEventStageUpdated {
	return crmcontracts.PublicEventStageUpdated{
		PipelineId:     openapi_types.UUID(pipelineID.UUID),
		Name:           in.Name,
		Semantic:       in.Semantic,
		WinProbability: committedWinProbability(in),
	}
}

func readStage(ctx context.Context, tx pgx.Tx, id ids.StageID, archived storekit.ArchivedFilter) (crmcontracts.Stage, error) {
	q := `SELECT id, pipeline_id, name, position, semantic, win_probability, created_at, updated_at, archived_at
	      FROM stage WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND archived_at IS NULL`
	}
	var out crmcontracts.Stage
	var stageID, pipelineID ids.UUID
	err := tx.QueryRow(ctx, q, id).Scan(&stageID, &pipelineID, &out.Name, &out.Position,
		&out.Semantic, &out.WinProbability, &out.CreatedAt, &out.UpdatedAt, &out.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Stage{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Stage{}, err
	}
	out.Id = openapi_types.UUID(stageID)
	out.PipelineId = openapi_types.UUID(pipelineID)
	return out, nil
}
