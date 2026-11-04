// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A role change reaches the member's mailbox on the next sweep. The wired seam
// makes the connection due at once, not at the end of its wait.
func TestChangingARoleMakesTheMembersMailboxDueNow(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	owner := integration.OwnerConn(t)
	conn := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'gmail', $2, 'connected')`, e.Rep1)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, next_sync_at) VALUES ($1, now() + interval '1 day')`, conn); err != nil {
		t.Fatalf("seeding the mailbox: %v", err)
	}
	service := identity.NewService(e.Pool).WithMailCapture(mailCaptureSeam{})

	if err := service.ChangeUserRole(e.Admin(), recoveryAdminIdentity(e), ids.From[ids.UserKind](e.Rep1), "read_only"); err != nil {
		t.Fatalf("changing the role: %v", err)
	}
	var next time.Time
	if err := owner.QueryRow(context.Background(),
		`SELECT next_sync_at FROM capture_sync_state WHERE connection_id = $1`, conn).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if time.Until(next) > time.Minute {
		t.Fatalf("next sync in %s after the role change, want due now", time.Until(next))
	}
}
