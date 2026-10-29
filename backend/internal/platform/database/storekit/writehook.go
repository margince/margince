// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AuditedWrite is one write as its audit row records it.
type AuditedWrite struct {
	Action     string
	EntityType string
	EntityID   ids.UUID
	Before     json.RawMessage
	After      json.RawMessage
}

// WriteHook runs after every audited write, inside the write's transaction.
// An error it returns rolls the whole write back.
type WriteHook func(ctx context.Context, tx pgx.Tx, w AuditedWrite) error

var writeHook atomic.Pointer[WriteHook]

// SetWriteHook installs the hook every audited write runs. Compose sets the
// extension set's dispatcher once at boot; unset, nothing runs.
func SetWriteHook(h WriteHook) {
	writeHook.Store(&h)
}

type inWriteHookKey struct{}

// runWriteHook runs the installed hook for one write. Writes the hook makes
// itself are audited without running it again, so a hook cannot recurse.
func runWriteHook(ctx context.Context, tx pgx.Tx, w AuditedWrite) error {
	h := writeHook.Load()
	if h == nil || ctx.Value(inWriteHookKey{}) != nil {
		return nil
	}
	return (*h)(context.WithValue(ctx, inWriteHookKey{}, true), tx, w)
}
