// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package teamchat holds the workspace-wide provider boundary shared by accounts and channels.
package teamchat

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The team services share one selection across personal and bot credentials.
const (
	Dispact = "dispact"
	Slack   = "slack"
	Teams   = "teams"
)

var services = []string{Dispact, Slack, Teams}

// Supported distinguishes record collaboration from unrelated customer messaging channels.
func Supported(provider string) bool { return slices.Contains(services, provider) }

// Selection binds cleanup to the administrative choice that requested it.
type Selection struct {
	Provider string
	Version  int64
}

// ReadSelection returns a zero version before an administrative choice exists.
func ReadSelection(ctx context.Context, tx pgx.Tx) (Selection, error) {
	var out Selection
	err := tx.QueryRow(ctx, "SELECT coalesce(provider,''),version FROM chat_service WHERE singleton").Scan(&out.Provider, &out.Version)
	if err == pgx.ErrNoRows {
		return Selection{}, nil
	}
	return out, err
}

// Admit serializes credential writers; reads use Check without taking this lock.
func Admit(ctx context.Context, tx pgx.Tx, provider string) error {
	if !Supported(provider) {
		return nil
	}
	if err := Lock(ctx, tx); err != nil {
		return err
	}
	return Check(ctx, tx, provider)
}

// Check enforces the selection without locking read-only requests.
func Check(ctx context.Context, tx pgx.Tx, provider string) error {
	if !Supported(provider) {
		return nil
	}
	selected, err := ReadSelection(ctx, tx)
	if err != nil {
		return err
	}
	if selected.Version > 0 {
		if selected.Provider == provider {
			return nil
		}
		return fmt.Errorf("select this team messaging service in workspace settings before connecting: %w", apperrors.ErrConflict)
	}
	providers, err := Providers(ctx, tx)
	if err != nil {
		return err
	}
	for _, current := range providers {
		if current != provider {
			return fmt.Errorf("select the workspace team messaging service before connecting another: %w", apperrors.ErrConflict)
		}
	}
	return nil
}

// HoldSelection prevents an earlier switch from retiring a later choice's credentials or links.
func HoldSelection(ctx context.Context, tx pgx.Tx, expected Selection) error {
	if err := Lock(ctx, tx); err != nil {
		return err
	}
	current, err := ReadSelection(ctx, tx)
	if err != nil {
		return err
	}
	if current != expected {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// Providers reads both owners so personal consent cannot bypass the shared connection boundary.
func Providers(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT provider FROM channel_connection
 WHERE provider=ANY(@services) AND archived_at IS NULL
 UNION SELECT provider FROM chat_member_grant WHERE status<>'disconnected'
 UNION SELECT provider FROM chat_link WHERE archived_at IS NULL
 UNION SELECT provider FROM chat_service WHERE provider IS NOT NULL
 ORDER BY provider`, pgx.StrictNamedArgs{"services": services})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Lock coordinates administrative selection with both credential writers.
func Lock(ctx context.Context, tx pgx.Tx) error {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(@key,0))", pgx.StrictNamedArgs{"key": "team-chat-provider:" + workspace.String()}); err != nil {
		return err
	}
	return nil
}
