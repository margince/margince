// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	threadLockLimit    = 128
	threadKeysArgument = "keys"
)

// Metadata and imports share the established thread-hold identity namespace.
const (
	threadComponent = `hashtextextended('capture_thread_hold_write:' || key, 0)`
	threadBarrier   = `hashtextextended('thread_merge', 0)`
)

type threadLockState struct {
	wide        bool
	held, fresh []int64
}

// WaitThreadKeysTx belongs before thread row locks. Wide work never queues an
// exclusive barrier behind ordinary transactions; it retries instead.
func WaitThreadKeysTx(ctx context.Context, tx pgx.Tx, keys []string) error {
	keys = threadLockKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	state, err := threadStateTx(ctx, tx, keys)
	if err != nil || state.wide || len(state.fresh) == 0 {
		return err
	}
	if len(state.held) > threadLockLimit {
		return tryWideThreadLock(ctx, tx)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(`+threadBarrier+`)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(key) FROM unnest(@keys::bigint[]) AS keys(key) ORDER BY key`, pgx.NamedArgs{threadKeysArgument: state.fresh}); err != nil {
		return err
	}
	return rememberThreadKeys(ctx, tx, state.held)
}

// LockThreadKeysTx never waits on components while its caller may own rows.
// The transaction-local ledger bounds cumulative locks across sibling writers.
func LockThreadKeysTx(ctx context.Context, tx pgx.Tx, keys ...string) error {
	keys = threadLockKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	state, err := threadStateTx(ctx, tx, keys)
	if err != nil || state.wide || len(state.fresh) == 0 {
		return err
	}
	if len(state.held) > threadLockLimit {
		return tryWideThreadLock(ctx, tx)
	}
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock_shared(`+threadBarrier+`)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return RetryThreadTransaction(ctx, tx)
	}
	if err := tx.QueryRow(ctx, `SELECT bool_and(pg_try_advisory_xact_lock(key)) FROM unnest(@keys::bigint[]) AS keys(key)`, pgx.NamedArgs{threadKeysArgument: state.fresh}).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return RetryThreadTransaction(ctx, tx)
	}
	return rememberThreadKeys(ctx, tx, state.held)
}

// Native transaction IDs prevent a pooled connection's old marker granting
// ownership. SET LOCAL and advisory locks both follow savepoint rollback.
func threadStateTx(ctx context.Context, tx pgx.Tx, keys []string) (threadLockState, error) {
	var state threadLockState
	err := tx.QueryRow(ctx, `WITH state AS MATERIALIZED (
 SELECT coalesce(current_setting('margince.thread_barrier_xid', true)=pg_current_xact_id_if_assigned()::text, false) AS wide,
 CASE WHEN current_setting('margince.thread_keys_xid', true)=pg_current_xact_id_if_assigned()::text
 THEN coalesce(nullif(current_setting('margince.thread_keys', true), ''), '{}')::bigint[] ELSE ARRAY[]::bigint[] END AS owned
 ), wanted AS MATERIALIZED (
 SELECT DISTINCT `+threadComponent+` AS key FROM unnest(@keys::text[]) AS keys(key)
 ) SELECT wide, ARRAY(SELECT key FROM wanted UNION SELECT unnest(owned) ORDER BY 1),
 ARRAY(SELECT key FROM wanted WHERE NOT key=ANY(owned) ORDER BY key) FROM state`, pgx.NamedArgs{threadKeysArgument: keys}).Scan(&state.wide, &state.held, &state.fresh)
	return state, err
}

func rememberThreadKeys(ctx context.Context, tx pgx.Tx, keys []int64) error {
	_, err := tx.Exec(ctx, `SELECT set_config('margince.thread_keys_xid', pg_current_xact_id()::text, true),
 set_config('margince.thread_keys', @keys::bigint[]::text, true)`, pgx.NamedArgs{threadKeysArgument: keys})
	return err
}

func tryWideThreadLock(ctx context.Context, tx pgx.Tx) error {
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(`+threadBarrier+`)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return RetryThreadTransaction(ctx, tx)
	}
	_, err := tx.Exec(ctx, `SELECT set_config('margince.thread_barrier_xid', pg_current_xact_id()::text, true)`)
	return err
}

// RetryThreadTransaction also poisons an ambient transaction, so catching its
// serialization error cannot commit a partially rewritten component.
func RetryThreadTransaction(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `DO $$ BEGIN RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'thread component changed; retry the transaction'; END $$`)
	if err == nil {
		return errors.New("thread component retry did not abort its transaction")
	}
	return err
}

func threadLockKeys(keys []string) []string {
	keys = slices.Clone(keys)
	slices.Sort(keys)
	return slices.Compact(slices.DeleteFunc(keys, func(key string) bool { return key == "" }))
}

// LockActivityThreadTx pins the row's current component before a sibling writes
// metadata. A contended component aborts instead of waiting on its merger.
func LockActivityThreadTx(ctx context.Context, tx pgx.Tx, activity ids.UUID) (string, error) {
	var key string
	if err := tx.QueryRow(ctx, `SELECT coalesce(thread_key, '') FROM activity WHERE id=@activity FOR SHARE`, pgx.NamedArgs{"activity": activity}).Scan(&key); err != nil {
		return "", err
	}
	if err := LockThreadKeysTx(ctx, tx, key); err != nil {
		return "", err
	}
	return key, nil
}
