// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readStages is the one assembly of a Stage, so every response carries the
// same version and counts. where filters `stage s` and binds against args.
func readStages(ctx context.Context, tx pgx.Tx, where string, args []any) ([]crmcontracts.Stage, error) {
	dealCount, err := visibleDealCount(ctx, &args, "d.stage_id = s.id")
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.pipeline_id, s.name, s.position, s.semantic, s.win_probability,
		       s.created_at, s.updated_at, s.archived_at, s.version,
		       crit.required, crit.optional, `+dealCount+`
		  FROM stage s
		 CROSS JOIN LATERAL (
		       SELECT count(*) FILTER (WHERE c.required) AS required,
		              count(*) FILTER (WHERE NOT c.required) AS optional
		         FROM stage_exit_criterion c
		        WHERE c.stage_id = s.id AND c.archived_at IS NULL) crit
		 WHERE `+where+`
		 ORDER BY s.pipeline_id, s.position`, args...)
	if err != nil {
		return nil, fmt.Errorf("read stages: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.Stage{}
	for rows.Next() {
		stage, err := scanStage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stages: %w", err)
	}
	return out, nil
}

func scanStage(rows pgx.Rows) (crmcontracts.Stage, error) {
	var out crmcontracts.Stage
	var stageID, pipelineID ids.UUID
	var version crmcontracts.RowVersion
	var criteria crmcontracts.StageExitCriteriaCounts
	err := rows.Scan(&stageID, &pipelineID, &out.Name, &out.Position, &out.Semantic, &out.WinProbability,
		&out.CreatedAt, &out.UpdatedAt, &out.ArchivedAt, &version,
		&criteria.Required, &criteria.Optional, &out.DealCount)
	if err != nil {
		return out, fmt.Errorf("scan stage: %w", err)
	}
	out.Id = openapi_types.UUID(stageID)
	out.PipelineId = openapi_types.UUID(pipelineID)
	out.Version = &version
	out.ExitCriteria = &criteria
	return out, nil
}

func readStage(ctx context.Context, tx pgx.Tx, id ids.StageID, archived storekit.ArchivedFilter) (crmcontracts.Stage, error) {
	where := `s.id = $1`
	if archived == storekit.LiveOnly {
		where += ` AND s.archived_at IS NULL`
	}
	stages, err := readStages(ctx, tx, where, []any{id})
	if err != nil {
		return crmcontracts.Stage{}, err
	}
	if len(stages) == 0 {
		return crmcontracts.Stage{}, apperrors.ErrNotFound
	}
	return stages[0], nil
}
