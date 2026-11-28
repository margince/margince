// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrManyLiveWorkspaces means the database holds more than one live company,
// which an installation never does. An operator keeps one company and archives
// the rest; nothing resolves the choice automatically.
var ErrManyLiveWorkspaces = errors.New("database: more than one active workspace; the single-company invariant requires an operator-led migration")

// ErrNotBootstrapped means the database holds no live company. The API
// refuses to serve until it has bootstrapped one, and the worker retries until
// then.
var ErrNotBootstrapped = errors.New("database: installation not bootstrapped; no active company exists")

// InstallationWorkspace is LiveWorkspace for the caller that needs a company
// to exist: no live row is ErrNotBootstrapped. It is the one lookup behind
// every handle's Workspace. Nothing caches its answer, so an archived or
// replaced company is seen on the next call.
func InstallationWorkspace(ctx context.Context, pool *pgxpool.Pool) (ids.UUID, error) {
	if pool == nil {
		return ids.UUID{}, fmt.Errorf("%w: no pool was injected", ErrNoWorkspace)
	}
	// pool-exempt: installation lookup. This reads the live workspace before any transaction exists.
	return installationFrom(ctx, pool)
}

// InstallationWorkspaceTx is InstallationWorkspace on the caller's transaction,
// so the answer and the statements that follow it see one snapshot.
func InstallationWorkspaceTx(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	return installationFrom(ctx, tx)
}

// ProvenWorkspace names the installation's one live company for a caller that
// holds a pool and no handle. It answers from the proof every handle on pool
// shares, so a read path that opens no transaction pays one lookup per
// InstallationRecheck. It fails when no company is live, or more than one is.
func ProvenWorkspace(ctx context.Context, pool *pgxpool.Pool) (ids.WorkspaceID, error) {
	if pool == nil {
		return ids.WorkspaceID{}, fmt.Errorf("%w: no pool was injected", ErrNoWorkspace)
	}
	return ForInstallation(pool).Workspace(ctx)
}

// installationWorkspaceMemo is the transaction-memo key of the live company.
// Every reader in one transaction shares it.
const installationWorkspaceMemo = "database:installation-workspace"

// WorkspaceTx names the installation's one live company inside tx. A store
// calls it for an audit entity id, a storage key or a job envelope.
// A transaction on an installation handle has already answered, so the call
// costs nothing there. Any other transaction looks it up once. It fails with
// ErrNotBootstrapped when no company is live.
func WorkspaceTx(ctx context.Context, tx pgx.Tx) (ids.WorkspaceID, error) {
	live, err := Remember(tx, installationWorkspaceMemo, func() (ids.UUID, error) {
		return InstallationWorkspaceTx(ctx, tx)
	})
	if err != nil {
		return ids.WorkspaceID{}, err
	}
	return ids.From[ids.WorkspaceKind](live), nil
}

// LiveWorkspace returns the installation's one live workspace, or ok=false
// when no company exists yet. It is the single read a pass over the
// installation starts from.
func LiveWorkspace(ctx context.Context, pool *pgxpool.Pool) (ws ids.UUID, ok bool, err error) {
	// pool-exempt: installation lookup. This reads the live workspace before any transaction exists.
	return liveFrom(ctx, pool)
}

type liveQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func installationFrom(ctx context.Context, q liveQuerier) (ids.UUID, error) {
	ws, ok, err := liveFrom(ctx, q)
	if err != nil {
		return ids.UUID{}, err
	}
	if !ok {
		return ids.UUID{}, ErrNotBootstrapped
	}
	return ws, nil
}

func liveFrom(ctx context.Context, q liveQuerier) (ids.UUID, bool, error) {
	rows, err := q.Query(ctx, `SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at LIMIT 2`)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("database: reading the live workspace: %w", err)
	}
	live, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("database: reading the live workspace: %w", err)
	}
	switch len(live) {
	case 0:
		return ids.UUID{}, false, nil
	case 1:
		return live[0], true, nil
	}
	return ids.UUID{}, false, ErrManyLiveWorkspaces
}

// EveryWorkspace lists every workspace row, archived ones included, oldest
// first. A privacy cleanup uses it. Archiving a workspace does not un-store its
// data, so a retention floor still binds there. The list is not empty merely
// because no company is live.
func EveryWorkspace(ctx context.Context, pool *pgxpool.Pool) ([]ids.UUID, error) {
	// pool-exempt: installation lookup. This reads every workspace row, archived included, before any transaction exists.
	rows, err := pool.Query(ctx, `SELECT id FROM workspace ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("database: listing every workspace: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
