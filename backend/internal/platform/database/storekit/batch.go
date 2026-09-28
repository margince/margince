// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

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
