// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a departed member's still-connected mailbox proves about an import.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A connection stays connected after its holder leaves, until somebody takes it
// down. The label on it then attributes imports to a mailbox nobody holds.
func TestADepartedSeatsMailboxProvesNothing(t *testing.T) {
	e := integration.Setup(t)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'rep@margince.test')`, e.Rep1); err != nil {
		t.Fatal(err)
	}

	// While the seat is live, so the fixture is known to produce a label at all.
	if labels := provedLabels(t, e)[e.Rep1]; len(labels) != 1 {
		t.Fatalf("a live seat's connected mailbox gave %d labels, want 1", len(labels))
	}

	e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)

	if labels := provedLabels(t, e)[e.Rep1]; len(labels) != 0 {
		t.Errorf("a departed seat's mailbox still proved %v", labels)
	}
}

func provedLabels(t *testing.T, e *integration.Env) map[ids.UUID][]string {
	t.Helper()
	var out map[ids.UUID][]string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = storekit.ProvedMailboxLabels(context.Background(), tx)
		return err
	}); err != nil {
		t.Fatalf("reading the proved mailbox labels: %v", err)
	}
	return out
}
