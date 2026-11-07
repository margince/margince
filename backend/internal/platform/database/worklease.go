// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type workLeaseKey struct{}
type workLeaseProof struct {
	pool  *pgxpool.Pool
	key   string
	nonce string
	lease *SessionLease
}

func (d *DB) workKey(ctx context.Context, kind string, id ids.UUID) (string, error) {
	workspace, err := d.Workspace(ctx)
	if err != nil {
		return "", err
	}
	return kind + ":" + workspace.String() + ":" + id.String(), nil
}

func (d *DB) TryWorkLease(ctx context.Context, kind string, id ids.UUID, log *slog.Logger) (*SessionLease, bool, error) {
	key, err := d.workKey(ctx, kind, id)
	if err != nil {
		return nil, false, err
	}
	return TrySessionLease(ctx, d.Pool(), key, log)
}

// WithWorkLease keeps ownership through model calls and their detached closing writes.
func (d *DB) WithWorkLease(ctx context.Context, kind string, id ids.UUID, log *slog.Logger, work func(context.Context) error) (bool, error) {
	key, err := d.workKey(ctx, kind, id)
	if err != nil {
		return false, err
	}
	lease, held, err := TrySessionLease(ctx, d.Pool(), key, log)
	if err != nil || !held {
		return held, err
	}
	defer lease.Close(ctx)
	nonce := "work:execution:" + ids.NewV7().String()
	if held, err = lease.TryLock(ctx, key, true); err != nil || !held {
		return false, err
	}
	if held, err = lease.TryLock(ctx, nonce, false); err != nil || !held {
		return false, err
	}
	if err := lease.Unlock(ctx, key, false); err != nil {
		return false, err
	}
	proof := workLeaseProof{pool: d.Pool(), key: key, nonce: nonce, lease: lease}
	owned := context.WithValue(ctx, workLeaseKey{}, proof)
	return true, work(WithTransactionGuard(owned, proof.guard))
}

// WorkLeaseOwned is false after release and for another tenant, pool or unit.
func (d *DB) WorkLeaseOwned(ctx context.Context, kind string, id ids.UUID) (bool, error) {
	proof, ok := ctx.Value(workLeaseKey{}).(workLeaseProof)
	if !ok || proof.pool != d.Pool() {
		return false, nil
	}
	key, err := d.workKey(ctx, kind, id)
	if err != nil {
		return false, err
	}
	if key != proof.key {
		return false, nil
	}
	proof.lease.mu.Lock()
	closed := proof.lease.conn == nil
	proof.lease.mu.Unlock()
	if closed {
		return false, nil
	}
	if err := proof.lease.Ping(ctx); err != nil {
		return false, err
	}
	proof.lease.mu.Lock()
	defer proof.lease.mu.Unlock()
	return proof.lease.conn != nil && slices.Contains(proof.lease.keys, sessionKey{name: key, shared: true}), nil
}

// WorkLeaseAvailable observes the same key without briefly blocking its worker.
func (d *DB) WorkLeaseAvailable(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (bool, error) {
	key, err := d.workKey(ctx, kind, id)
	if err != nil {
		return false, err
	}
	var free bool
	err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_locks
 WHERE locktype = 'advisory' AND granted AND objsubid = 1
 AND ((classid::bigint << 32) | (objid::bigint & 4294967295)) = hashtextextended(@key, 0))`, pgx.StrictNamedArgs{"key": key}).Scan(&free)
	return free, err
}

// The transaction keeps the unit shared while the nonce proves its execution still exists.
// A replacement must take the unit exclusively, so it cannot overtake an admitted write.
func (p workLeaseProof) guard(ctx context.Context, tx pgx.Tx) error {
	var owned bool
	err := tx.QueryRow(ctx, `WITH unit AS MATERIALIZED (
  SELECT pg_advisory_xact_lock_shared(hashtextextended(@unit, 0)))
  SELECT NOT pg_try_advisory_xact_lock_shared(hashtextextended(@nonce, 0)) FROM unit`,
		pgx.StrictNamedArgs{"unit": p.key, "nonce": p.nonce}).Scan(&owned)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("%w: work execution no longer owns its session", apperrors.ErrConflict)
	}
	return nil
}
