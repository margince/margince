// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type batchKey struct{}

// WithBatch marks every audit row written under ctx as part of one bulk change.
//
// It travels on the context for the reason extension attribution does: a bulk
// change drives each module's own single-record write, and threading a batch
// parameter through every one of those writers would be a parameter each new
// writer has to remember.
func WithBatch(ctx context.Context, batchID ids.UUID) context.Context {
	return context.WithValue(ctx, batchKey{}, batchID)
}

// batchOf answers the batch an audit row belongs to, or nil for a write made on
// its own.
func batchOf(ctx context.Context) *ids.UUID {
	id, ok := ctx.Value(batchKey{}).(ids.UUID)
	if !ok || id == ids.Nil {
		return nil
	}
	return &id
}

type importRunKey struct{}

// WithImportRun marks every write under ctx as landed by one data import run.
// It is not WithBatch, because a bulk change is a batch too. Only an import's
// events must tell their consumers that the history they carry is not new.
func WithImportRun(ctx context.Context, runID ids.UUID) context.Context {
	return context.WithValue(ctx, importRunKey{}, runID)
}

// ImportRunOf answers the import run the write belongs to, or nil for a write
// made on its own.
func ImportRunOf(ctx context.Context) *ids.UUID {
	id, ok := ctx.Value(importRunKey{}).(ids.UUID)
	if !ok || id == ids.Nil {
		return nil
	}
	return &id
}

// ImportMark is the pair of members an event of an import's write carries, so
// an outside system can tell brought-in history from new work. Both are nil for
// a write made on its own, which keeps the members absent from the payload.
func ImportMark(ctx context.Context) (imported *bool, runID *openapi_types.UUID) {
	run := ImportRunOf(ctx)
	if run == nil {
		return nil, nil
	}
	yes := true
	id := openapi_types.UUID(*run)
	return &yes, &id
}
