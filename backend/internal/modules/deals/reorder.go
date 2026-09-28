// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reordering, named whole: a pipeline's stage ladder, and the pipeline catalog.
// Each takes the complete order in one write rather than a position per record,
// because uq_stage_position is checked row by row and a run of single moves
// passes through states the index refuses — dragging the third stage to the top
// collides with the first on the very first step.
//
// The ladder's shape is one rule held by every writer of it: won and lost come
// after every open stage, because the board reads the ladder top to bottom and
// a closing stage above an open one puts the end of a deal in its middle.

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	codeOrderStale             = "order_stale"
	codeOrderDuplicate         = "duplicate"
	codeClosingStageBeforeOpen = "closing_stage_before_open"
	codePositionOutOfRange     = "out_of_range"

	// The column a ladder and a catalog order by, as the audit images and the
	// refusals name it, and the key a reorder's position delta rides under.
	positionField       = "position"
	stagePositionsField = "stage_positions"
	// A new pipeline's opening stages, as a create refuses them.
	stagesField = "stages"
)

// lockLadder takes the pipeline row every ladder write serializes on, and
// answers ErrNotFound for one that is gone or retired. NO KEY, because nothing
// a ladder write changes on the row is a key: a deal created into the pipeline
// takes FOR KEY SHARE on it for its foreign key and need not queue behind a
// reorder, while two ladder writes still queue behind each other — and behind a
// retirement, a promotion or a catalog reorder, which take FOR UPDATE.
func lockLadder(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	var locked ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM pipeline WHERE id = $1 AND archived_at IS NULL FOR NO KEY UPDATE`,
		pipelineID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock the pipeline's ladder: %w", err)
	}
	return nil
}

// OrderStaleError refuses an order that does not name exactly the live records
// it reorders. The caller drew it from a read somebody has since changed — a
// record added or retired — so the way forward is to read again. It unwraps to
// ErrConflict, so it answers 409: nothing in the request is wrong on its own.
type OrderStaleError struct {
	Noun string
}

func (e *OrderStaleError) Error() string { return "the " + e.Noun + " order is stale" }

func (e *OrderStaleError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault carries the refusal's verdict for the REST mapper and the
// datasource seam alike.
func (e *OrderStaleError) MessageFault() (code, message string) {
	return codeOrderStale, "the order must name every live " + e.Noun +
		" exactly once, and this one no longer does; reload and try again"
}

// ranked is one live record of an order and the position it holds now.
// `closing` marks a won or lost stage; a pipeline in the catalog has none.
type ranked struct {
	id       ids.UUID
	position int
	closing  bool
}

// arrange answers current in the order the caller named, refusing a list that
// names a record twice or does not name exactly the records current holds.
func arrange(current []ranked, wanted []ids.UUID, field, noun string) ([]ranked, error) {
	byID := make(map[ids.UUID]ranked, len(current))
	for _, r := range current {
		byID[r.id] = r
	}
	seen := make(map[ids.UUID]bool, len(wanted))
	out := make([]ranked, 0, len(wanted))
	for _, id := range wanted {
		if seen[id] {
			return nil, &values.ParseError{
				Field: field, Code: codeOrderDuplicate,
				Message: "names the same " + noun + " more than once; list each once",
			}
		}
		seen[id] = true
		r, ok := byID[id]
		if !ok {
			return nil, &OrderStaleError{Noun: noun}
		}
		out = append(out, r)
	}
	if len(out) != len(current) {
		return nil, &OrderStaleError{Noun: noun}
	}
	return out, nil
}

// placement is one record an order moves: from the position it held to its
// rank in the new order.
type placement struct {
	id       ids.UUID
	from, to int
}

// placements answers the records whose rank in order differs from the position
// they hold. A record already at its rank is left alone, so an order that moves
// nothing writes nothing.
func placements(order []ranked) []placement {
	var out []placement
	for i, r := range order {
		if r.position != i+1 {
			out = append(out, placement{id: r.id, from: r.position, to: i + 1})
		}
	}
	return out
}

// positionImages are the audit before- and after-images of a reorder, and the
// after-image is also the event's delta, so the trail and the wire body name the
// same moves.
func positionImages(moves []placement) (before, after map[string]any) {
	before = make(map[string]any, len(moves))
	after = make(map[string]any, len(moves))
	for _, m := range moves {
		before[m.id.String()] = m.from
		after[m.id.String()] = m.to
	}
	return before, after
}

// readLadder answers the pipeline's live stages in position order, locked to be
// rewritten under the pipeline lock the caller holds. NO KEY, because a position
// is no key a deal's stage_id refers by: a deal moving onto one of these stages
// takes FOR KEY SHARE and need not wait for its position to change.
func readLadder(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) ([]ranked, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, position, semantic <> 'open' FROM stage
		 WHERE pipeline_id = $1 AND archived_at IS NULL
		 ORDER BY position FOR NO KEY UPDATE`, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("read the pipeline's stages: %w", err)
	}
	defer rows.Close()
	var ladder []ranked
	for rows.Next() {
		var r ranked
		if err := rows.Scan(&r.id, &r.position, &r.closing); err != nil {
			return nil, fmt.Errorf("scan a stage: %w", err)
		}
		ladder = append(ladder, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the pipeline's stages: %w", err)
	}
	return ladder, nil
}

// placeStages writes the moves in two set-based statements, each safe in any
// row order: the first parks every mover strictly below every position the
// ladder holds, so no park meets a live row; the second takes each to its rank,
// which no stayer holds (a stayer holds its own rank) and no other mover holds
// (it is parked, or at its own rank).
func placeStages(ctx context.Context, tx pgx.Tx, ladder []ranked, moves []placement) error {
	if len(moves) == 0 {
		return nil
	}
	floor := 1
	for _, r := range ladder {
		floor = min(floor, r.position)
	}
	if floor-len(moves) <= math.MinInt32 {
		return &values.ParseError{
			Field: positionField, Code: codePositionOutOfRange,
			Message: "a stage sits too low to reorder around; give it a small position first",
		}
	}
	movers := make([]ids.UUID, len(moves))
	parks := make([]int, len(moves))
	ranks := make([]int, len(moves))
	for i, m := range moves {
		movers[i], parks[i], ranks[i] = m.id, floor-1-i, m.to
	}
	const place = `UPDATE stage SET position = v.position
		FROM unnest($1::uuid[], $2::int[]) AS v(id, position) WHERE stage.id = v.id`
	if _, err := tx.Exec(ctx, place, movers, parks); err != nil {
		return fmt.Errorf("park the moving stages: %w", err)
	}
	if _, err := tx.Exec(ctx, place, movers, ranks); err != nil {
		return fmt.Errorf("place the moving stages: %w", err)
	}
	return nil
}

// markLadderChanged moves the pipeline's version, which is its ladder's version:
// an order is guarded by it, so every write that adds, removes or repositions a
// stage moves it. The row trigger does the bump; the caller holds the lock.
func markLadderChanged(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE pipeline SET updated_at = now() WHERE id = $1 AND archived_at IS NULL`, pipelineID); err != nil {
		return fmt.Errorf("move the pipeline's version: %w", err)
	}
	return nil
}

// emitStagePositions publishes a ladder's moves as ONE pipeline.updated — a
// reorder is a fact about the pipeline, never N stage.updated.
func emitStagePositions(ctx context.Context, tx pgx.Tx, auditID ids.UUID, pipelineID ids.PipelineID, after map[string]any) error {
	if err := storekit.EmitEvent(ctx, tx, auditID, pipelineID.UUID, crmcontracts.PublicEventPipelineUpdated{
		ChangedFields: map[string]any{stagePositionsField: after},
	}); err != nil {
		return fmt.Errorf("emit pipeline reorder: %w", err)
	}
	return nil
}

// ReorderStages puts a pipeline's live stages in the order named, as positions
// 1..n, refusing an order that puts a closing stage above an open one.
func (s *Store) ReorderStages(ctx context.Context, pipelineID ids.PipelineID, stageIDs []ids.UUID, ifVersion *int64) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockLadder(ctx, tx, pipelineID); err != nil {
			return err
		}
		current, err := readPipelineConfig(ctx, tx, pipelineID)
		if err != nil {
			return err
		}
		if ifVersion != nil && *ifVersion != current.version {
			return apperrors.ErrVersionSkew
		}
		ladder, err := readLadder(ctx, tx, pipelineID)
		if err != nil {
			return err
		}
		order, err := arrange(ladder, stageIDs, "stage_ids", "stage")
		if err != nil {
			return err
		}
		if err := refuseClosingBeforeOpen(order, "stage_ids"); err != nil {
			return err
		}
		if moves := placements(order); len(moves) > 0 {
			if err := writeStageOrder(ctx, tx, pipelineID, ladder, moves); err != nil {
				return err
			}
		}
		if out, err = readPipeline(ctx, tx, pipelineID); err != nil {
			return fmt.Errorf("read reordered pipeline: %w", err)
		}
		return nil
	})
	return out, err
}

// writeStageOrder commits the moves, the ladder's version, the audit row and
// the one pipeline.updated carrying the position delta.
func writeStageOrder(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID, ladder []ranked, moves []placement) error {
	if err := placeStages(ctx, tx, ladder, moves); err != nil {
		return err
	}
	if err := markLadderChanged(ctx, tx, pipelineID); err != nil {
		return err
	}
	before, after := positionImages(moves)
	auditID, err := storekit.Audit(ctx, tx, "update", "pipeline", pipelineID.UUID,
		map[string]any{stagePositionsField: before}, map[string]any{stagePositionsField: after})
	if err != nil {
		return fmt.Errorf("audit stage reorder: %w", err)
	}
	return emitStagePositions(ctx, tx, auditID, pipelineID, after)
}

// ReorderPipelines puts the live pipelines in the order named. Pipeline
// positions carry no uniqueness, so each mover goes straight to its rank.
func (s *Store) ReorderPipelines(ctx context.Context, pipelineIDs []ids.UUID) ([]crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return nil, err
	}
	var out []crmcontracts.Pipeline
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		catalog, err := lockCatalog(ctx, tx)
		if err != nil {
			return err
		}
		order, err := arrange(catalog, pipelineIDs, "pipeline_ids", "pipeline")
		if err != nil {
			return err
		}
		for _, m := range placements(order) {
			if err := movePipeline(ctx, tx, m); err != nil {
				return err
			}
		}
		out, err = listPipelinesTx(ctx, tx, storekit.LiveOnly)
		return err
	})
	return out, err
}

// lockCatalog answers the live pipelines, each locked, in id order. Every write
// that locks more than one pipeline takes them in that order — this, and
// lockPromotion — so any two of them queue rather than deadlock.
func lockCatalog(ctx context.Context, tx pgx.Tx) ([]ranked, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, position FROM pipeline WHERE archived_at IS NULL ORDER BY id FOR UPDATE`)
	if err != nil {
		return nil, fmt.Errorf("lock the pipelines: %w", err)
	}
	defer rows.Close()
	var catalog []ranked
	for rows.Next() {
		var r ranked
		if err := rows.Scan(&r.id, &r.position); err != nil {
			return nil, fmt.Errorf("scan a pipeline: %w", err)
		}
		catalog = append(catalog, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lock the pipelines: %w", err)
	}
	return catalog, nil
}

// lockPromotion locks the pipeline being made default and the incumbent it
// demotes, in id order, before either is written — the order lockCatalog
// takes them in.
func lockPromotion(ctx context.Context, tx pgx.Tx, id ids.PipelineID) error {
	if _, err := tx.Exec(ctx,
		`SELECT id FROM pipeline WHERE id = $1 OR (is_default AND archived_at IS NULL)
		 ORDER BY id FOR UPDATE`, id); err != nil {
		return fmt.Errorf("lock the pipelines a promotion touches: %w", err)
	}
	return nil
}

// movePipeline commits one pipeline's new position with its own audit row and
// pipeline.updated, the same fact a PATCH of its position records.
func movePipeline(ctx context.Context, tx pgx.Tx, m placement) error {
	if _, err := tx.Exec(ctx,
		`UPDATE pipeline SET position = $2 WHERE id = $1 AND archived_at IS NULL`, m.id, m.to); err != nil {
		return fmt.Errorf("move pipeline: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "pipeline", m.id,
		map[string]any{positionField: m.from}, map[string]any{positionField: m.to})
	if err != nil {
		return fmt.Errorf("audit pipeline move: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, m.id, crmcontracts.PublicEventPipelineUpdated{
		ChangedFields: map[string]any{positionField: m.to},
	}); err != nil {
		return fmt.Errorf("emit pipeline move: %w", err)
	}
	return nil
}
