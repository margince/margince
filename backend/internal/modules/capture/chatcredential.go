// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChatCredentials reads the access token a connected chat account was sealed
// with, for the ingest worker that downloads a message's media.
type ChatCredentials struct {
	Pool  *pgxpool.Pool
	Vault keyvault.Vault
}

// WhatsAppAccessToken unseals the access token of one connected number. The
// context carries the workspace the job runs for.
func (r ChatCredentials) WhatsAppAccessToken(ctx context.Context, connection ids.UUID) (string, error) {
	company, err := database.ProvenWorkspace(ctx, r.Pool)
	if r.Vault == nil || err != nil {
		return "", ErrChannelWiringIncomplete
	}
	ws := company.UUID
	var token string
	err = database.WithTx(ctx, r.Pool, func(tx pgx.Tx) error {
		var ref string
		if err := tx.QueryRow(ctx, `SELECT credential_ref FROM channel_connection
			 WHERE id = $1 AND archived_at IS NULL AND `+whatsappRow, connection).Scan(&ref); err != nil {
			return err
		}
		sealed, err := r.Vault.GetOn(ctx, tx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(ref))
		token = string(sealed)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("capture: the whatsapp connection is gone: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("capture: reading the whatsapp access token: %w", err)
	}
	return token, nil
}
