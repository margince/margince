// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refuseAgentSemanticChange keeps which stages close a deal with a user.
// Automatic work needs approval to move a deal into a won or lost stage. An agent
// that could relabel a stage, or add a closing one, would lift that for every agent
// working unattended.
func refuseAgentSemanticChange(ctx context.Context, current string, wanted *string) error {
	if wanted == nil || *wanted == current {
		return nil
	}
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	return &httperr.DetailedError{
		Status: http.StatusForbidden, Code: "agent_sets_no_stage_semantic",
		Detail: "an agent does not decide whether a stage counts as won or lost; ask the user to do it in the app",
	}
}

// refuseAgentOnTrustedLadder keeps a ladder still while a transition on it may move
// deals by itself. That move is judged by its stage's criteria and by which stage
// comes next. An agent reshaping either would lift the check it runs under. The
// caller holds the ladder's trust lock from the start of its transaction.
func refuseAgentOnTrustedLadder(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	var trusted bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM stage_progression_policy p
		    JOIN stage from_stage ON from_stage.id = p.from_stage_id AND from_stage.archived_at IS NULL
		    JOIN stage to_stage ON to_stage.id = p.to_stage_id AND to_stage.archived_at IS NULL
		   WHERE p.pipeline_id = $1 AND p.mode = 'auto')`, pipelineID).Scan(&trusted); err != nil {
		return fmt.Errorf("read whether a transition is trusted: %w", err)
	}
	if !trusted {
		return nil
	}
	return &httperr.DetailedError{
		Status: http.StatusForbidden, Code: "agent_reshapes_no_trusted_ladder",
		Detail: "a transition on this pipeline is trusted to move deals by itself, so an agent does not change its stages or criteria; ask the user to do it in the app",
	}
}

// refuseAgentOnStagesLadder is refuseAgentOnTrustedLadder for a write that names a stage.
func refuseAgentOnStagesLadder(ctx context.Context, tx pgx.Tx, stageID ids.StageID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	var pipelineID ids.PipelineID
	if err := tx.QueryRow(ctx, `SELECT pipeline_id FROM stage WHERE id = $1`, stageID).Scan(&pipelineID); err != nil {
		return fmt.Errorf("read the stage's pipeline: %w", err)
	}
	return refuseAgentOnTrustedLadder(ctx, tx, pipelineID)
}

// lockLadderTrust orders an agent's reshape against a transition becoming trusted,
// so neither commits on a reading the other has just made false. Each takes it
// before any row. A stage row held while waiting here would deadlock with the
// policy write, whose foreign key check reads that row under this lock.
func lockLadderTrust(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	return storekit.LockWriteIdentity(ctx, tx, "stage_ladder_trust", pipelineID.String())
}

// holdLadderForAgent opens an agent's write on a ladder. A user's write takes no
// such lock, because no trusted-ladder refusal applies to it.
func holdLadderForAgent(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	return lockLadderTrust(ctx, tx, pipelineID)
}

// holdStagesLadderForAgent is holdLadderForAgent for a write that names a stage. It
// reads without a lock. A stage that does not exist is the write's own refusal.
func holdStagesLadderForAgent(ctx context.Context, tx pgx.Tx, stageID ids.StageID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	var pipelineID ids.PipelineID
	err := tx.QueryRow(ctx, `SELECT pipeline_id FROM stage WHERE id = $1`, stageID).Scan(&pipelineID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the stage's pipeline: %w", err)
	}
	return lockLadderTrust(ctx, tx, pipelineID)
}

// stagePositionTaken says which position is occupied and what frees it.
func stagePositionTaken(position int) error {
	return &httperr.DetailedError{
		Status: http.StatusConflict, Code: "stage_position_taken",
		Detail: fmt.Sprintf("position %d is already taken in this pipeline; choose a free position, or reorder the stages to put this one there", position),
	}
}
