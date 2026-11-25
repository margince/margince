// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The rule that a pipeline always keeps a way to win and a way to lose a deal.
// Removal already refuses a closing stage, so retyping one is the only way
// left to drop below the pair.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// The names the seeded default ladder gives its closing stages.
const (
	defaultWonStageName  = "Won"
	defaultLostStageName = "Lost"
)

// closingPairFloor is the lowest position a supplied closing stage takes. It
// leaves the positions below free for the open stages added later.
const closingPairFloor = 1000

// missingClosingStages answers the default Won and Lost stage the ladder lacks.
// Each goes behind the last stage and the floor, named as the seeded ladder
// names them. Creation and the repair of an existing pipeline both ask it.
func missingClosingStages(stages []StageInput) []StageInput {
	has := map[StageSemantic]bool{}
	last := closingPairFloor - 1
	for _, st := range stages {
		has[StageSemantic(st.Semantic)] = true
		last = max(last, st.Position)
	}
	var missing []StageInput
	for _, closing := range []StageInput{
		{Name: defaultWonStageName, Semantic: string(SemanticWon), WinProbability: 100, NameTranslations: seededTranslations(defaultWonStageName)},
		{Name: defaultLostStageName, Semantic: string(SemanticLost), WinProbability: 0, NameTranslations: seededTranslations(defaultLostStageName)},
	} {
		if has[StageSemantic(closing.Semantic)] {
			continue
		}
		last++
		closing.Position = last
		missing = append(missing, closing)
	}
	return missing
}

// withClosingPair answers the ladder with the Won and Lost stage it lacks
// appended. A pipeline is never born unable to close a deal, and a client that
// sends only open stages keeps working.
func withClosingPair(stages []StageInput) []StageInput {
	return append(slices.Clone(stages), missingClosingStages(stages)...)
}

// ensureClosingPair adds the Won or Lost stage a live pipeline lacks, which an
// older version let an admin retype away. It runs on restore and whenever a
// ladder write takes the ladder lock, so a pipeline is repaired the first time
// anyone edits it. A ladder that holds both stages
// costs one read and no write.
func ensureClosingPair(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	rows, err := tx.Query(ctx,
		`SELECT semantic, position FROM stage WHERE pipeline_id = $1 AND archived_at IS NULL`, pipelineID)
	if err != nil {
		return fmt.Errorf("read the ladder to check its closing stages: %w", err)
	}
	live, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (StageInput, error) {
		var st StageInput
		err := row.Scan(&st.Semantic, &st.Position)
		return st, err
	})
	if err != nil {
		return fmt.Errorf("read the ladder to check its closing stages: %w", err)
	}
	for _, st := range missingClosingStages(live) {
		if err := insertClosingStage(ctx, tx, pipelineID, st); err != nil {
			return err
		}
	}
	return nil
}

func insertClosingStage(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID, st StageInput) error {
	var stageID ids.StageID
	if err := tx.QueryRow(ctx, `
		INSERT INTO stage (pipeline_id, name, position, semantic, win_probability, name_translations)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		pipelineID, st.Name, st.Position, st.Semantic, st.WinProbability, st.NameTranslations).Scan(&stageID); err != nil {
		return fmt.Errorf("insert the missing closing stage: %w", err)
	}
	if err := markLadderChanged(ctx, tx, pipelineID); err != nil {
		return err
	}
	image := stageCreateImage(pipelineID, st.Name, st.Semantic, st.NameTranslations)
	image[seededTranslationsMark] = true
	auditID, err := storekit.Audit(ctx, tx, "create", "stage", stageID.UUID, nil, image)
	if err != nil {
		return fmt.Errorf("audit the missing closing stage: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, stageID.UUID,
		stageCreatedPayload(pipelineID, st.Name, st.Position, st.Semantic, st.WinProbability))
}
