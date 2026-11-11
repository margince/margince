// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// These receipts have their own contract; the general bulk reader leaves them out.
const correctionUndoVerb = "undo_corrections"
const correctionUndoColumns = "id, params, result, requested_by, requested_for, created_at"

type correctionUndoParams struct {
	Items []crmcontracts.UndoMarginceChangeItem `json:"items"`
}

type correctionUndoResults struct {
	Results []crmcontracts.UndoMarginceChangeResult `json:"results"`
}

type correctionUndoRun struct {
	id        ids.UUID
	params    correctionUndoParams
	result    correctionUndoResults
	requester batchRequester
	createdAt time.Time
}

func scanCorrectionUndoRun(row pgx.Row) (correctionUndoRun, error) {
	var run correctionUndoRun
	var params, result []byte
	if err := row.Scan(&run.id, &params, &result, &run.requester.id, &run.requester.human, &run.createdAt); err != nil {
		return run, err
	}
	if err := json.Unmarshal(params, &run.params); err != nil {
		return run, fmt.Errorf("read correction Undo items: %w", err)
	}
	if err := json.Unmarshal(result, &run.result); err != nil {
		return run, fmt.Errorf("read correction Undo results: %w", err)
	}
	return run, nil
}

func lockedCorrectionUndoRun(ctx context.Context, tx pgx.Tx, id ids.UUID) (correctionUndoRun, error) {
	run, err := scanCorrectionUndoRun(tx.QueryRow(ctx, "SELECT "+correctionUndoColumns+` FROM bulk_operation WHERE id=@id AND verb=@verb FOR UPDATE`, pgx.NamedArgs{"id": id, "verb": correctionUndoVerb}))
	if errors.Is(err, pgx.ErrNoRows) {
		return run, apperrors.ErrNotFound
	}
	if err != nil {
		return run, err
	}
	// Only the requesting human may resume; the read door also admits administrators.
	actor, present := principal.Actor(ctx)
	if !present || actor.ID != run.requester.id || run.requester.human == nil || humanBehind(actor) != *run.requester.human {
		return run, apperrors.ErrNotFound
	}
	if len(run.result.Results) > len(run.params.Items) {
		return run, fmt.Errorf("stored correction Undo results exceed its items")
	}
	return run, nil
}

func (h marginceChangeHandlers) startCorrectionUndoRun(ctx context.Context, id ids.UUID, items []crmcontracts.UndoMarginceChangeItem) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	params, err := json.Marshal(correctionUndoParams{Items: items})
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO bulk_operation (id, record_type, verb, params, requested_by, requested_for, changed_count, skipped_count, result)
 VALUES (@id, 'deal', @verb, @params, @actor, @human, 0, 0, '{"results":[]}'::jsonb)
 ON CONFLICT (id) DO NOTHING`, pgx.NamedArgs{"id": id, "verb": correctionUndoVerb, "params": params, "actor": actor.ID, "human": storekit.UUIDOrNil(humanBehind(actor))})
		if err != nil {
			return err
		}
		run, err := lockedCorrectionUndoRun(ctx, tx, id)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(run.params.Items, items) {
			return apperrors.ErrConflict
		}
		return nil
	})
}

func writeCorrectionUndoResult(ctx context.Context, tx pgx.Tx, run correctionUndoRun) error {
	result, err := json.Marshal(run.result)
	if err != nil {
		return err
	}
	changed := 0
	for _, item := range run.result.Results {
		if len(item.Undone) > 0 {
			changed++
		}
	}
	_, err = tx.Exec(ctx, `UPDATE bulk_operation SET result=@result, changed_count=@changed, skipped_count=@skipped
 WHERE id=@id AND verb=@verb`, pgx.NamedArgs{"result": result, "changed": changed, "skipped": len(run.result.Results) - changed, "id": run.id, "verb": correctionUndoVerb})
	return err
}
