// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// Managing the accounting source: connecting one, disconnecting it, and
// linking its customers onto companies. All of it is an administrator's act
// under the `integrations` object; the `finance` object stays read-only, so no
// grant on it reaches a write (FIN-DDL-N-1).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	objectIntegrations         = "integrations"
	statusConnectionConnecting = "connecting"
	statusDisconnected         = "disconnected"
	// connectionWriteIdentity serializes every writer of the one live
	// connection, so two administrators connecting at once make one row.
	connectionWriteIdentity = "live"
)

// WithSources binds the sources this build can connect.
func (s *Store) WithSources(sources Sources) *Store {
	s.sources = sources
	return s
}

// SourceState answers the connected source, if any, and the ones on offer.
func (s *Store) SourceState(ctx context.Context) (crmcontracts.FinanceSourceState, error) {
	if err := auth.Require(ctx, objectIntegrations, principal.ActionRead); err != nil {
		return crmcontracts.FinanceSourceState{}, err
	}
	var out crmcontracts.FinanceSourceState
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.readSourceState(ctx, tx)
		return err
	})
	return out, err
}

func (s *Store) readSourceState(ctx context.Context, tx pgx.Tx) (crmcontracts.FinanceSourceState, error) {
	out := crmcontracts.FinanceSourceState{Available: []crmcontracts.FinanceSourceOption{}}
	if s.sources != nil {
		for _, option := range s.sources.Providers() {
			out.Available = append(out.Available, crmcontracts.FinanceSourceOption{
				Provider: option.Provider, Description: option.Description,
			})
		}
	}
	var conn crmcontracts.FinanceConnection
	err := tx.QueryRow(ctx, `
		SELECT id, provider, status, created_at, last_attempt_at, last_success_at, last_error_code
		  FROM finance_connection
		 WHERE archived_at IS NULL AND status <> 'disconnected'
		 ORDER BY created_at DESC
		 LIMIT 1`).Scan(&conn.Id, &conn.Provider, &conn.Status, &conn.ConnectedAt,
		&conn.LastAttemptAt, &conn.LastSuccessAt, &conn.LastErrorCode)
	if err == nil {
		out.Connection = &conn
		return out, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, fmt.Errorf("read the finance connection: %w", err)
}

// Connect asks the source whether it can be read, then records it as the
// workspace's one live connection. Connecting the source already connected is
// answered unchanged. A different one waits for a disconnect: the mirror reads
// one source, and two would interleave their ledgers.
func (s *Store) Connect(ctx context.Context, provider string) (crmcontracts.FinanceSourceState, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.FinanceSourceState{}, err
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionCreate); err != nil {
		return crmcontracts.FinanceSourceState{}, err
	}
	id := ids.NewV7()
	conn := SourceConnection{ID: id.String(), ConnectedAt: s.now().UTC()}
	reader, known := s.open(provider, conn)
	if !known {
		return crmcontracts.FinanceSourceState{}, httperr.Validation("provider", "unsupported",
			fmt.Sprintf("no accounting source %q is installed", provider))
	}
	// Asked before any row exists, so a source that cannot be read leaves
	// nothing behind for the sweep to fail on every six hours.
	if err := reader.Check(ctx); err != nil {
		return crmcontracts.FinanceSourceState{}, fmt.Errorf("%w: %s cannot be read: %w", apperrors.ErrProviderUnusable, provider, err)
	}
	var out crmcontracts.FinanceSourceState
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, entityConnection, connectionWriteIdentity); err != nil {
			return err
		}
		live, connected, err := readConnection(ctx, tx)
		if err != nil {
			return err
		}
		switch {
		case connected && live.provider != provider:
			return fmt.Errorf("%w: %s is connected; disconnect it before connecting %s", apperrors.ErrConflict, live.provider, provider)
		case !connected:
			if err := insertConnection(ctx, tx, id, conn.ConnectedAt, provider); err != nil {
				return err
			}
		}
		out, err = s.readSourceState(ctx, tx)
		return err
	})
	return out, err
}

func insertConnection(ctx context.Context, tx pgx.Tx, id ids.UUID, connectedAt time.Time, provider string) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// credential_ref names where the credential lives: with the unit that
	// supplies the source, never in this table.
	if _, err := tx.Exec(ctx, `
		INSERT INTO finance_connection (id, provider, status, credential_ref, created_at, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'manual', $6)`,
		id, provider, statusConnectionConnecting, "extension:"+provider, connectedAt, by); err != nil {
		return fmt.Errorf("record the finance connection: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", entityConnection, id, nil,
		connectionImage{Status: statusConnectionConnecting}); err != nil {
		return fmt.Errorf("audit the finance connection: %w", err)
	}
	return nil
}

// Disconnect stops the sync. The mirrored rows stay, labelled with their
// source, so the history a reader came for is not lost with the connection.
func (s *Store) Disconnect(ctx context.Context) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, entityConnection, connectionWriteIdentity); err != nil {
			return err
		}
		live, connected, err := readConnection(ctx, tx)
		if err != nil || !connected {
			return err
		}
		before, err := readConnectionState(ctx, tx, live.id)
		if err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRow(ctx,
			`SELECT version FROM finance_connection WHERE id = $1 FOR UPDATE`, live.id).Scan(&version); err != nil {
			return fmt.Errorf("read the finance connection: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE finance_connection SET status = $2
			 WHERE id = $1 AND version = $3 AND archived_at IS NULL`,
			live.id, statusDisconnected, version)
		if err != nil {
			return fmt.Errorf("disconnect the finance connection: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: the connection changed meanwhile; reload and try again", apperrors.ErrConflict)
		}
		return auditConnectionTransition(ctx, tx, live.id, before,
			connectionImage{Status: statusDisconnected, ErrorCode: before.ErrorCode})
	})
}

// RequireConnected answers ErrNotFound when no source is connected, for a
// caller about to queue a sync that would find nothing to do.
func (s *Store) RequireConnected(ctx context.Context) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, connected, err := readConnection(ctx, tx)
		if err == nil && !connected {
			return fmt.Errorf("%w: no accounting source is connected", apperrors.ErrNotFound)
		}
		return err
	})
}

//nolint:ireturn // the seam IS an interface: which source reads this connection
func (s *Store) open(provider string, conn SourceConnection) (Provider, bool) {
	if s.sources == nil {
		return nil, false
	}
	return s.sources.Open(provider, conn)
}
