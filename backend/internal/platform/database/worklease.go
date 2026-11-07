// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type workLeaseKey struct{}
type workLeaseProof struct {
	pool  *pgxpool.Pool
	key   string
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
	owned := context.WithValue(ctx, workLeaseKey{}, workLeaseProof{pool: d.Pool(), key: key, lease: lease})
	return true, work(owned)
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
	return proof.lease.conn != nil && slices.Contains(proof.lease.keys, sessionKey{name: key}), nil
}

// WorkLeaseAvailable checks the same key without reserving a worker's session slot.
func (d *DB) WorkLeaseAvailable(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (bool, error) {
	key, err := d.workKey(ctx, kind, id)
	if err != nil {
		return false, err
	}
	var free bool
	err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended(@key, 0))`, pgx.StrictNamedArgs{"key": key}).Scan(&free)
	return free, err
}
