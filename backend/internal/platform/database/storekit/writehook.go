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
// extension set's dispatcher once at boot; unset or nil, nothing runs.
func SetWriteHook(h WriteHook) {
	if h == nil {
		writeHook.Store(nil)
		return
	}
	writeHook.Store(&h)
}

type inWriteHookKey struct{}

// WithinWriteHook marks ctx as running inside a write hook. A context that
// carries the mark runs no hook on its own writes.
func WithinWriteHook(ctx context.Context) context.Context {
	return context.WithValue(ctx, inWriteHookKey{}, true)
}

// InWriteHook reports whether ctx carries the write-hook mark.
func InWriteHook(ctx context.Context) bool {
	return ctx.Value(inWriteHookKey{}) != nil
}

// runWriteHook runs the installed hook for one write. Writes the hook makes
// itself are audited without running it again, so a hook cannot recurse.
func runWriteHook(ctx context.Context, tx pgx.Tx, w AuditedWrite) error {
	h := writeHook.Load()
	if h == nil || InWriteHook(ctx) {
		return nil
	}
	return (*h)(WithinWriteHook(ctx), tx, w)
}
