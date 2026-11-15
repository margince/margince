// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package snapshotreads remembers successful reads for the length of one
// read-only unit of work in one snapshot transaction.
//
// A reporting evaluation resolves the same named team or owner many times: for
// the context, for each metric under its own savepoint, for the targets. Each
// resolution reads the subject's label, and that read is also the check that
// the subject is still live. Inside one repeatable-read or serializable
// transaction every repeat returns the same row, so the first successful
// answer can stand for the rest.
//
// The memo keeps that guarantee narrow:
//   - It lives only between Open and its release, on the context Open returns.
//   - It answers only on the connection of the transaction it was opened on.
//     Savepoints share that connection and snapshot. Another transaction does not.
//   - It answers only under repeatable read or serializable. Under read
//     committed every call reads again, so a caller's isolation is never weakened.
//   - It keeps successes only. An error, a missing row or a cancelled context
//     is never remembered.
//
// It is for reads of rows the unit of work does not write. A caller that
// writes what it remembers must not open one.
//
// database.Remember does not fit: it is keyed by the pgx.Tx value, so each
// metric's savepoint misses it, and it lasts the whole transaction under any
// isolation, so every writer of users and teams would have to call Forget.
package snapshotreads

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type memo struct {
	root     pgx.Tx
	closed   bool
	checked  bool
	snapshot bool
	values   map[any]any
}

type memoKey struct{}

// Open starts a memo for one unit of work on tx. A context that already holds
// an open memo on the same connection keeps it, and the returned release then
// leaves it to the caller that opened it.
func Open(ctx context.Context, tx pgx.Tx) (context.Context, func()) {
	if open, ok := ctx.Value(memoKey{}).(*memo); ok && open.usableOn(tx) {
		return ctx, func() {}
	}
	m := &memo{root: tx, values: make(map[any]any)}
	return context.WithValue(ctx, memoKey{}, m), func() {
		m.closed = true
		m.values = nil
	}
}

func (m *memo) usableOn(tx pgx.Tx) bool {
	return !m.closed && m.root != nil && tx != nil && m.root.Conn() == tx.Conn()
}

// Read answers the remembered value for key, or runs read and remembers its
// value when it succeeds. Key must be comparable; a caller's own key type keeps
// its entries apart from every other caller's. Without a usable memo, Read
// just runs read.
func Read[V any](ctx context.Context, tx pgx.Tx, key any, read func() (V, error)) (V, error) {
	m, ok := ctx.Value(memoKey{}).(*memo)
	if !ok || ctx.Err() != nil || !m.usableOn(tx) {
		return read()
	}
	if !m.checked {
		var isolation string
		if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation')`).Scan(&isolation); err != nil {
			var zero V
			return zero, fmt.Errorf("snapshotreads: reading the transaction isolation: %w", err)
		}
		m.snapshot = isolation == "repeatable read" || isolation == "serializable"
		m.checked = true
	}
	if !m.snapshot {
		return read()
	}
	if value, typed := m.values[key].(V); typed {
		return value, nil
	}
	value, err := read()
	if err == nil {
		m.values[key] = value
	}
	return value, err
}

// IDSet is a set of ids as one comparable key part: sorted, without
// duplicates. The ids passed in are not changed.
func IDSet(set []ids.UUID) string {
	keys := make([]string, len(set))
	for i, id := range set {
		keys[i] = id.String()
	}
	slices.Sort(keys)
	return strings.Join(slices.Compact(keys), ",")
}
