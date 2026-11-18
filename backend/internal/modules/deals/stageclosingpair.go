// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The rule that a pipeline always keeps a way to win and a way to lose a deal.
// Removal already refuses a closing stage, so retyping one is the only way
// left to drop below the pair.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// codeLastClosingStage is what a write answers when it would leave a pipeline
// without a live won or a live lost stage.
const codeLastClosingStage = "last_closing_stage"

func lastClosingStageError(semantic StageSemantic) error {
	kind := strings.ToUpper(string(semantic[:1])) + string(semantic[1:])
	return &values.ParseError{
		Field: stageSemanticField, Code: codeLastClosingStage,
		Message: fmt.Sprintf("A pipeline needs a %[1]s stage to close deals as %[1]s. Add another %[1]s stage first.", kind),
	}
}

// refuseLastClosingStageRetype stops a stage that is its pipeline's only live
// won (or lost) stage from changing type. Runs under the ladder lock, so two
// retypes of the pair's two members queue instead of both passing.
func refuseLastClosingStageRetype(
	ctx context.Context, tx pgx.Tx, id ids.StageID, pipelineID ids.PipelineID, currentSemantic string, wanted *string,
) error {
	if wanted == nil || *wanted == currentSemantic || !StageSemantic(currentSemantic).Terminal() {
		return nil
	}
	var others int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM stage
		 WHERE pipeline_id = $1 AND semantic = $2 AND id <> $3 AND archived_at IS NULL`,
		pipelineID, currentSemantic, id).Scan(&others); err != nil {
		return fmt.Errorf("count the pipeline's other %s stages: %w", currentSemantic, err)
	}
	if others > 0 {
		return nil
	}
	return lastClosingStageError(StageSemantic(currentSemantic))
}
