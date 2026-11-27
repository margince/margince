// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// txMemo holds what one transaction has read for reuse inside it. It is keyed
// by the transaction itself, not carried on a context. The stores read with
// the context their caller passed into the transaction.
type txMemo struct {
	mu     sync.Mutex
	values map[string]any
	marks  map[string]struct{}
	after  []func()
	before []func(context.Context, pgx.Tx) error
}

var txMemos sync.Map

// CommitOwner returns the database-owned transaction that will commit tx.
// Savepoints share its write bookkeeping, while their read memos stay separate.
//
//nolint:ireturn // pgx.Tx is the driver's transaction interface.
func CommitOwner(tx pgx.Tx) pgx.Tx {
	for {
		if owned, ok := tx.(*ownedTx); ok {
			return owned.root
		}
		if wrapped, ok := tx.(interface{ Unwrap() pgx.Tx }); ok {
			tx = wrapped.Unwrap()
			continue
		}
		return tx
	}
}

// ownedTx carries commit ownership through nested pgx savepoints.
type ownedTx struct {
	pgx.Tx
	root pgx.Tx
}

// Unwrap answers the driver's transaction, preserving separate read memos.
//
//nolint:ireturn // pgx.Tx is the driver's transaction interface.
func (t *ownedTx) Unwrap() pgx.Tx { return t.Tx }

//nolint:ireturn // pgx.Tx is the driver's transaction interface.
func (t *ownedTx) Begin(ctx context.Context) (pgx.Tx, error) {
	child, err := t.Tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &ownedTx{Tx: child, root: t.root}, nil
}

// openTxMemo gives tx a memo for its lifetime; the returned func ends it. A
// savepoint's own pgx.Tx is a different value, so it has none and reads fresh.
func openTxMemo(tx pgx.Tx) func() {
	memo := &txMemo{values: make(map[string]any), marks: make(map[string]struct{})}
	txMemos.Store(tx, memo)
	return func() {
		txMemos.Delete(tx)
		memo.mu.Lock()
		after := memo.after
		memo.mu.Unlock()
		for _, fn := range after {
			fn()
		}
	}
}

// AfterTx runs fn once tx has ended, committed or not, and reports whether it
// was registered. A transaction this package did not open, or a savepoint,
// has no end to hook, and fn is not kept.
func AfterTx(tx pgx.Tx, fn func()) bool {
	memo, ok := memoOf(tx)
	if !ok {
		return false
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	memo.after = append(memo.after, fn)
	return true
}

func memoOf(tx pgx.Tx) (*txMemo, bool) {
	held, ok := txMemos.Load(underlying(tx))
	if !ok {
		return nil, false
	}
	memo, ok := held.(*txMemo)
	return memo, ok
}

// Remember answers read's value, running it once per key in tx. It suits only
// a read that no lock this transaction relies on protects. A writer in the same
// transaction calls Forget for the key it changes.
// The caller must not mutate the value it gets back. Outside a transaction this
// package opened, read runs every time.
func Remember[T any](tx pgx.Tx, key string, read func() (T, error)) (T, error) {
	memo, ok := memoOf(tx)
	if !ok {
		return read()
	}
	memo.mu.Lock()
	held, hit := memo.values[key]
	memo.mu.Unlock()
	if value, typed := held.(T); hit && typed {
		return value, nil
	}
	value, err := read()
	if err != nil {
		return value, err
	}
	memo.mu.Lock()
	memo.values[key] = value
	memo.mu.Unlock()
	return value, nil
}

// Forget drops every remembered value whose key starts with prefix, for a
// write in tx that changes what those reads answer.
func Forget(tx pgx.Tx, prefix string) {
	memo, ok := memoOf(tx)
	if !ok {
		return
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	for key := range memo.values {
		if strings.HasPrefix(key, prefix) {
			delete(memo.values, key)
		}
	}
}

// Mark notes key as having happened in tx and reports whether it is the first
// time. Outside a transaction this package opened, every call is the first.
func Mark(tx pgx.Tx, key string) bool {
	memo, ok := memoOf(tx)
	if !ok {
		return true
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if _, held := memo.marks[key]; held {
		return false
	}
	memo.marks[key] = struct{}{}
	return true
}

// Marked reports whether Mark noted key in tx.
func Marked(tx pgx.Tx, key string) bool {
	memo, ok := memoOf(tx)
	if !ok {
		return false
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	_, held := memo.marks[key]
	return held
}

// Memoized reports whether tx has a memo: a transaction this package opened,
// not a savepoint.
func Memoized(tx pgx.Tx) bool {
	_, ok := memoOf(tx)
	return ok
}

// BeforeCommit runs fn inside tx before it commits. Later registrations run first.
// Work registered by a callback is drained before an earlier callback runs.
func BeforeCommit(tx pgx.Tx, fn func(context.Context, pgx.Tx) error) bool {
	memo, ok := memoOf(tx)
	if !ok {
		return false
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	memo.before = append(memo.before, fn)
	return true
}

func runBeforeCommit(ctx context.Context, tx pgx.Tx) error {
	memo, ok := memoOf(tx)
	if !ok {
		return nil
	}
	for {
		memo.mu.Lock()
		count := len(memo.before)
		if count == 0 {
			memo.mu.Unlock()
			return nil
		}
		fn := memo.before[count-1]
		memo.before = memo.before[:count-1]
		memo.mu.Unlock()
		if err := fn(ctx, tx); err != nil {
			return err
		}
	}
}
