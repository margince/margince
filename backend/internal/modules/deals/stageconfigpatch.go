// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The pre-update reads and the patches built from them, for both config
// records. They live beside stages.go rather than inside it because the two
// share one obligation — an audit image is only honest if it comes from the row
// as this transaction found it — and because stages.go is already near the
// file-size ceiling.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pipelineConfig is the pre-update image of the three editable pipeline
// columns, plus the version an If-Match is judged against. One read serves
// both, under the row lock, so the concurrency verdict and the audit
// before-image describe the same row.
type pipelineConfig struct {
	name      string
	isDefault bool
	position  int
	version   int64
}

func readPipelineConfig(ctx context.Context, tx pgx.Tx, id ids.PipelineID) (pipelineConfig, error) {
	var current pipelineConfig
	err := tx.QueryRow(ctx,
		`SELECT name, is_default, position, version FROM pipeline WHERE id = $1 AND archived_at IS NULL`, id).
		Scan(&current.name, &current.isDefault, &current.position, &current.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return current, apperrors.ErrNotFound
	}
	if err != nil {
		return current, fmt.Errorf("read pipeline before update: %w", err)
	}
	return current, nil
}

// pipelineUpdatePatch folds the caller's sparse update onto the row that was
// just read. A nil pointer is an omitted field, not a change to null, so an
// untouched column appears in neither audit image nor in changed_fields.
func pipelineUpdatePatch(current pipelineConfig, in UpdatePipelineInput) *storekit.Patch {
	patch := storekit.NewPatch()
	if in.Name != nil {
		patch.Set("name", current.name, *in.Name)
	}
	if in.IsDefault != nil {
		patch.Set("is_default", current.isDefault, *in.IsDefault)
	}
	if in.Position != nil {
		patch.Set(positionField, current.position, *in.Position)
	}
	return patch
}

// stageConfig is the pre-update image of the four editable stage columns, plus
// the version an If-Match is judged against.
type stageConfig struct {
	name           string
	position       int
	semantic       string
	winProbability int
	version        int64
}

func readStageConfig(ctx context.Context, tx pgx.Tx, id ids.StageID) (stageConfig, error) {
	var current stageConfig
	err := tx.QueryRow(ctx, `
		SELECT name, position, semantic, win_probability, version
		FROM stage WHERE id = $1 AND archived_at IS NULL`, id).
		Scan(&current.name, &current.position, &current.semantic, &current.winProbability, &current.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return current, apperrors.ErrNotFound
	}
	if err != nil {
		return current, fmt.Errorf("read stage before update: %w", err)
	}
	return current, nil
}

// terminalWinProbability is the odds the stage_terminal_prob CHECK pins a won
// or lost stage to, and false for an open stage, whose odds are the caller's.
// Every writer of a stage's odds reads it here, so a terminal stage outranks
// whatever the caller sent the same way on create and on update.
func terminalWinProbability(semantic string) (int, bool) {
	switch StageSemantic(semantic) {
	case SemanticWon:
		return 100, true
	case SemanticLost:
		return 0, true
	}
	return 0, false
}

// stageProbability is the win probability a new stage is written with: a
// terminal stage's pinned odds, else the caller's, else 0.
func stageProbability(semantic string, given *int) int {
	if pinned, ok := terminalWinProbability(semantic); ok {
		return pinned
	}
	if given != nil {
		return *given
	}
	return 0
}

// committedWinProbability resolves the win_probability an update actually
// commits, or nil when it leaves the column where it was. Resolving it here and
// binding it as a plain value, rather than deriving it again inside the UPDATE,
// is what lets the row, the audit after-image and the published payload all
// report the number that was actually stored.
func committedWinProbability(in UpdateStageInput) *int {
	if in.Semantic != nil {
		if pinned, ok := terminalWinProbability(*in.Semantic); ok {
			return &pinned
		}
	}
	return in.WinProbability
}

// stageUpdatePatch folds the caller's sparse update onto the row that was just
// read, so the audit diff spans exactly the columns this save touches.
func stageUpdatePatch(current stageConfig, in UpdateStageInput) *storekit.Patch {
	patch := storekit.NewPatch()
	if in.Name != nil {
		patch.Set("name", current.name, *in.Name)
	}
	if in.Position != nil {
		patch.Set(positionField, current.position, *in.Position)
	}
	if in.Semantic != nil {
		patch.Set(stageSemanticField, current.semantic, *in.Semantic)
	}
	if probability := committedWinProbability(in); probability != nil {
		patch.Set("win_probability", current.winProbability, *probability)
	}
	return patch
}
