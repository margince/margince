// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SharedMailbox describes a shared mailbox connection: no seat owns it, an
// admin connected it, and one team reads what it captures.
type SharedMailbox struct {
	Provider    string
	Address     string
	ReaderTeam  ids.UUID
	ConnectedBy ids.UUID
	// Posture is held or shared; a shared mailbox is never classified.
	Posture string
}

// ConnectSharedMailbox writes the connection row the admin connect will write,
// and is the one place every suite makes a shared mailbox.
func ConnectSharedMailbox(t *testing.T, tx pgx.Tx, box SharedMailbox) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := tx.QueryRow(context.Background(), `
		INSERT INTO capture_connection (user_id, connected_by, reader_team_id, provider, account_label, mail_posture, status, scopes, auth)
		VALUES (NULL, $1, $2, $3, $4, $5, 'connected', '{read}', 'shared-mailbox-token')
		RETURNING id`,
		box.ConnectedBy, box.ReaderTeam, box.Provider, box.Address, box.Posture).Scan(&id); err != nil {
		t.Fatalf("connecting the shared mailbox: %v", err)
	}
	return id
}
