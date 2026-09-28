// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The shape every ladder write keeps: positions inside the parkable range, and
// the won and lost stages after every open one.

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// stagePositionCeiling bounds the position a stage may be written with. A
// reorder parks its movers just below the lowest position the ladder holds, so
// a position at the edge of the column's range would leave it nowhere to park.
const stagePositionCeiling = 1 << 20

func checkStagePosition(position int) error {
	if position < 0 || position > stagePositionCeiling {
		return &values.ParseError{
			Field: positionField, Code: codePositionOutOfRange,
			Message: fmt.Sprintf("position must be between 0 and %d", stagePositionCeiling),
		}
	}
	return nil
}

// checkNewLadder holds a pipeline's opening stages to the shape every later
// ladder write keeps: each position in range, none held twice, and the closing
// stages after the open ones in position order.
func checkNewLadder(stages []StageInput) error {
	ladder := make([]ranked, len(stages))
	held := make(map[int]bool, len(stages))
	for i, st := range stages {
		if err := checkStagePosition(st.Position); err != nil {
			return err
		}
		if held[st.Position] {
			return &values.ParseError{
				Field: stagesField, Code: codeOrderDuplicate,
				Message: fmt.Sprintf("two stages name position %d", st.Position),
			}
		}
		held[st.Position] = true
		ladder[i] = ranked{position: st.Position, closing: st.Semantic != string(SemanticOpen)}
	}
	slices.SortFunc(ladder, func(a, b ranked) int { return cmp.Compare(a.position, b.position) })
	return refuseClosingBeforeOpen(ladder, stagesField)
}

// openFirst answers the ladder with its open stages first and its closing
// stages after, each run keeping the order it had.
func openFirst(ladder []ranked) []ranked {
	out := make([]ranked, 0, len(ladder))
	for _, closing := range []bool{false, true} {
		for _, r := range ladder {
			if r.closing == closing {
				out = append(out, r)
			}
		}
	}
	return out
}

func refuseClosingBeforeOpen(order []ranked, field string) error {
	closed := false
	for _, r := range order {
		if r.closing {
			closed = true
			continue
		}
		if closed {
			return &values.ParseError{
				Field: field, Code: codeClosingStageBeforeOpen,
				Message: "won and lost stages close a deal, so they come after every open stage",
			}
		}
	}
	return nil
}

// putOpenFirst restores the ladder's shape after a write that can break it — a
// stage created past the closing pair, or a stage turned won or lost — moving
// the closing stages down behind the open ones. A ladder already in shape is left exactly as it is, positions
// and gaps included: a create keeps the position it asked for. The shift rides
// the caller's audit row as its own reorder.
func putOpenFirst(ctx context.Context, tx pgx.Tx, auditID ids.UUID, pipelineID ids.PipelineID) error {
	ladder, err := readLadder(ctx, tx, pipelineID)
	if err != nil {
		return err
	}
	if refuseClosingBeforeOpen(ladder, positionField) == nil {
		return nil
	}
	moves := placements(openFirst(ladder))
	if err := placeStages(ctx, tx, ladder, moves); err != nil {
		return err
	}
	_, after := positionImages(moves)
	return emitStagePositions(ctx, tx, auditID, pipelineID, after)
}

// holdLadderShape answers for an update that moved a stage or changed whether
// it closes a deal. A move that puts a closing stage above an open one is
// refused, since the caller named that place; a stage-type change is restored
// to shape instead, closing stages sliding behind the open ones. Either way
// the version moves, since an order drawn before this write no longer
// describes the ladder.
func holdLadderShape(ctx context.Context, tx pgx.Tx, auditID ids.UUID, pipelineID ids.PipelineID, current stageConfig, in UpdateStageInput) error {
	moved := in.Position != nil && *in.Position != current.position
	flipped := in.Semantic != nil && *in.Semantic != current.semantic
	switch {
	case moved:
		ladder, err := readLadder(ctx, tx, pipelineID)
		if err != nil {
			return err
		}
		if err := refuseClosingBeforeOpen(ladder, positionField); err != nil {
			return err
		}
	case flipped:
		if err := putOpenFirst(ctx, tx, auditID, pipelineID); err != nil {
			return err
		}
	default:
		return nil
	}
	return markLadderChanged(ctx, tx, pipelineID)
}
