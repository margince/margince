// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ModelRateKey names one entry of the price sheet: a model at a vendor, in the
// lane it is filed under. Every effective-dated price of the entry shares it.
type ModelRateKey struct {
	Provider string
	ModelID  string
	Lane     Lane
}

// DeleteModelRate removes a model's whole entry from the sheet, history
// included; its past calls read as unpriced afterwards, never as priced at 0.
// Update is the grant: a removal corrects the sheet, and no role holds delete.
func (s *RateStore) DeleteModelRate(ctx context.Context, key ModelRateKey) error {
	key, err := prepareModelRateKey(ctx, key)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return deleteModelRate(ctx, tx, key)
	})
}

// prepareModelRateKey runs the connection-free gates — RBAC admission and the
// key's shape — so a refused caller never costs a transaction.
func prepareModelRateKey(ctx context.Context, key ModelRateKey) (ModelRateKey, error) {
	if err := auth.Require(ctx, "ai_model_rate", principal.ActionUpdate); err != nil {
		return ModelRateKey{}, err
	}
	provider, modelID, err := modelRateIdentity(key.Provider, key.ModelID)
	if err != nil {
		return ModelRateKey{}, err
	}
	if !knownLane(key.Lane) {
		return ModelRateKey{}, unknownLane()
	}
	return ModelRateKey{Provider: provider, ModelID: modelID, Lane: key.Lane}, nil
}

// removedModelRate is one row the delete took, as the ledger records it.
type removedModelRate struct {
	id    ids.UUID
	image map[string]any
}

func deleteModelRate(ctx context.Context, tx pgx.Tx, key ModelRateKey) error {
	// Under the model's write-identity lock, so a price being written for the
	// same model lands wholly before or wholly after the entry goes.
	if err := storekit.LockWriteIdentity(ctx, tx, "ai_model_rate", modelRateLockKey(key.Provider, key.ModelID)); err != nil {
		return err
	}
	// The lane the sheet files the model under is its head row's (filedLane),
	// so the key is matched there and the entry then goes whole: a model
	// re-filed on a later date keeps no stray rows under its old lane.
	rows, err := tx.Query(ctx, `
		DELETE FROM ai_model_rate
		WHERE provider = $1 AND model_id = $2
		  AND $3 = (SELECT lane FROM ai_model_rate WHERE provider = $1 AND model_id = $2
		            ORDER BY effective_date DESC LIMIT 1)
		RETURNING id, input_per_mtok_microusd, output_per_mtok_microusd,
		          cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date, lane`,
		key.Provider, key.ModelID, string(key.Lane))
	if err != nil {
		return fmt.Errorf("delete ai_model_rate: %w", err)
	}
	removed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (removedModelRate, error) {
		var (
			id      ids.UUID
			gone    = preparedModelRate{provider: key.Provider, modelID: key.ModelID}
			effDate time.Time
		)
		err := row.Scan(&id, &gone.input, &gone.output, &gone.cacheRead, &gone.cacheWrite, &effDate, &gone.lane)
		return removedModelRate{id: id, image: modelRateImage(gone, effDate)}, err
	})
	if err != nil {
		return fmt.Errorf("scan deleted ai_model_rate: %w", err)
	}
	if len(removed) == 0 {
		return apperrors.ErrNotFound
	}
	// One ledger row per price the entry held, in the writer's own image, so the
	// history shows a removed price in the fields it was entered under.
	for _, r := range removed {
		if _, err := storekit.Audit(ctx, tx, "erase", "ai_model_rate", r.id, r.image, nil); err != nil {
			return fmt.Errorf("audit ai_model_rate erase: %w", err)
		}
	}
	return nil
}
