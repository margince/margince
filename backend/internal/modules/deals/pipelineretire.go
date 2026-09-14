// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Retiring a pipeline, and putting it back.
//
// A pipeline was the one piece of configuration a workspace could create and
// never retire: `archived_at` was on the table, the read filtered on it, the
// wire type exposed it and listPipelines declared `include_archived` — and no
// endpoint could produce the state. Every reorganisation of a sales process
// left its old pipelines in the picker permanently.
//
// ARCHIVING RETIRES A CHOICE, NOT WORK. A deal on an archived pipeline keeps its
// stage, its history and its forecast contribution, and its record page still
// renders the stage it is on. Forcing deals off first would turn retiring a
// pipeline into a bulk migration, and an operation nobody dares run retires
// nothing. It is also how archiving already behaves elsewhere here: it hides a
// record from day-to-day work, deletes nothing, and moves nothing attached.

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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// codeDefaultPipelineNotArchivable is what a retirement answers while the
// pipeline is still the installation's default.
const codeDefaultPipelineNotArchivable = "default_pipeline_not_archivable"

// ArchivePipeline retires a pipeline.
//
// The refusal is the interesting part: the DEFAULT cannot be retired while it is
// the default. An installation with no default has no answer for "where does a
// new deal go", and meeting that at deal-creation time is worse than being
// refused here.
//
// The refusal names the remedy — make another pipeline the default first —
// because clearing it must not need authority the caller lacks: promoting
// another pipeline is `updatePipeline`, the same `pipeline:update` grant this
// caller already holds to have got here. A refusal whose way forward the caller
// cannot reach is a wall rather than a door.
func (s *Store) ArchivePipeline(ctx context.Context, id ids.PipelineID, ifVersion *int64) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		// The pipeline row is the serialization point for every write that
		// reshapes it or its stage list (lockStageConfig's own doc), so a
		// retirement and a stage reorder queue rather than racing.
		if _, err := storekit.LockRow(ctx, tx, "pipeline", id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		current, err := readPipelineConfig(ctx, tx, id)
		if err != nil {
			return err
		}
		if ifVersion != nil && *ifVersion != current.version {
			return apperrors.ErrVersionSkew
		}
		// Read inside the lock, not from a snapshot taken before it: the
		// default moves by an ordinary update, and a check against a stale read
		// would retire the default a moment after somebody else promoted it.
		if current.isDefault {
			return &values.ParseError{
				Field: "", Code: codeDefaultPipelineNotArchivable,
				Message: "this is the default pipeline, and new deals need one; make another pipeline " +
					"the default first, then retire this one",
			}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE pipeline SET archived_at = $2 WHERE id = $1 AND archived_at IS NULL`,
			id, time.Now().UTC()); err != nil {
			return fmt.Errorf("archive pipeline: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "archive", "pipeline", id.UUID, nil, nil)
		if err != nil {
			return fmt.Errorf("audit pipeline archive: %w", err)
		}
		// The FIRST producer of pipeline.archived. The type was published with
		// its schema and no writer, so a subscriber could subscribe and never
		// hear anything; that is what this closes.
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
			crmcontracts.PublicEventPipelineArchived{}); err != nil {
			return fmt.Errorf("emit pipeline.archived: %w", err)
		}
		return nil
	})
}

// RestorePipeline puts a retired pipeline back in use.
//
// IDEMPOTENT BY PREDICATE: restoring a pipeline that is not archived changes
// nothing and answers the pipeline, so the retry after a lost response reads the
// same as the first call. The audit row is written only on the transition, so a
// second call records no decision nobody made.
//
// It does NOT make the pipeline default again. Archiving required the default to
// move elsewhere first, and putting a pipeline back is not a claim about where
// new deals should go — that is updatePipeline's to say, and inferring it here
// would silently demote whichever pipeline took over.
func (s *Store) RestorePipeline(ctx context.Context, id ids.PipelineID) (crmcontracts.Pipeline, error) {
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return crmcontracts.Pipeline{}, err
	}
	var out crmcontracts.Pipeline
	txErr := s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, "pipeline", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		// RETURNING the value it held, so the audit row records what the
		// restore replaced rather than only that one happened. A restore has a
		// prior state — that is what separates it from a create — and an image
		// pair naming nothing would make the trail unable to say when the
		// pipeline had been retired.
		var wasArchivedAt time.Time
		err := tx.QueryRow(ctx,
			`UPDATE pipeline SET archived_at = NULL
			  WHERE id = $1 AND archived_at IS NOT NULL
			 RETURNING (SELECT archived_at FROM pipeline WHERE id = $1)`, id).Scan(&wasArchivedAt)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Not archived: nothing to restore, and no decision to record. The
			// read below still answers the pipeline, so a retry after a lost
			// response reads the same as the call that landed.
		case err != nil:
			return fmt.Errorf("restore pipeline: %w", err)
		default:
			if _, err := storekit.Audit(ctx, tx, "restore", "pipeline", id.UUID,
				map[string]any{"archived_at": wasArchivedAt},
				map[string]any{"archived_at": nil}); err != nil {
				return fmt.Errorf("audit pipeline restore: %w", err)
			}
		}
		out, err = readPipeline(ctx, tx, id)
		if errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err != nil {
			return fmt.Errorf("read restored pipeline: %w", err)
		}
		return nil
	})
	return out, txErr
}
