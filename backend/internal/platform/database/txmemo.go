// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
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
}

var txMemos sync.Map

// openTxMemo gives tx a memo for its lifetime; the returned func ends it. A
// savepoint's own pgx.Tx is a different value, so it has none and reads fresh.
func openTxMemo(tx pgx.Tx) func() {
	txMemos.Store(tx, &txMemo{values: make(map[string]any)})
	return func() { txMemos.Delete(tx) }
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
