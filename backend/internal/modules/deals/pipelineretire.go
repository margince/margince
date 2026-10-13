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
//
// A restore can CONFLICT, which an archive cannot: pipeline_name_unique covers the
// live rows, so the name this pipeline held was free while it was retired and
// another pipeline may hold it now. That is answered as a conflict naming the
// holder, because the caller's way out is to rename one of the two and only they
// can say which.
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
		if err := nameStillFree(ctx, tx, id); err != nil {
			return err
		}
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
		case nameTakenMeanwhile(err):
			// The lock above holds THIS row; nothing stops another transaction
			// creating a live pipeline under the same name between the check and
			// this update. The index settles it, and the caller is owed the same
			// answer either way — which of the two gets renamed is still theirs.
			return errPipelineNameTaken
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

// errPipelineNameTaken says a retired pipeline cannot come back under its own name
// because a live pipeline now holds it.
var errPipelineNameTaken = fmt.Errorf("%w: a live pipeline already has this name", apperrors.ErrConflict)

// nameStillFree answers whether this pipeline's name is free for it to hold again.
//
// Checked rather than left to the index: the index raises 23505, which reaches the
// caller as a 500 saying nothing, and the caller is the only one who can choose
// which of the two pipelines gets renamed. Inside the row lock, so the answer cannot
// go stale between the read and the update.
//
// Excluding itself is what keeps the restore idempotent: a pipeline that is not
// archived is live, so it holds its own name, and without this it would read as its
// own conflict.
func nameStillFree(ctx context.Context, tx pgx.Tx, id ids.PipelineID) error {
	var taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pipeline live
			 WHERE live.archived_at IS NULL
			   AND live.id <> $1
			   AND live.name = (SELECT name FROM pipeline WHERE id = $1)
		)`, id).Scan(&taken); err != nil {
		return fmt.Errorf("check whether a restored pipeline's name is free: %w", err)
	}
	if taken {
		return errPipelineNameTaken
	}
	return nil
}

// nameTakenMeanwhile reports the index refusing a restore because the name went to a
// live pipeline after nameStillFree looked.
//
// By the constraint's name rather than the code alone: another unique on this table
// would otherwise read as this conflict and send the caller to rename a name that was
// never the problem.
func nameTakenMeanwhile(err error) bool {
	constraint, isUnique := storekit.UniqueViolation(err)
	return isUnique && constraint == "pipeline_name_unique"
}
