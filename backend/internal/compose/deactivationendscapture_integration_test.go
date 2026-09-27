// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Deactivating a seat has to stop their mailbox being read. Everything else the
// cascade ends is a credential this product issued; a capture connection holds
// the PROVIDER's, and the poller that spends it never asked whose seat it was.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The whole point: the connection is withdrawn and the sealed credential is
// gone, in the transaction that deactivated the seat.
func TestDeactivatingASeatWithdrawsTheirCaptureConnections(t *testing.T) {
	e := integration.Setup(t)
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))
	seedCaptureConnection(t, e, e.Rep1, "imap", []byte(`{"password":"live"}`))
	// A colleague's mailbox, to prove the withdrawal is bounded to the seat.
	seedCaptureConnection(t, e, e.Rep2, "gmail", []byte(`{"token":"untouched"}`))

	deactivate(t, e, e.Rep1)

	for _, provider := range []string{"gmail", "imap"} {
		status, auth := connectionState(t, e, e.Rep1, provider)
		if status != "disconnected" {
			t.Errorf("%s status = %q, want disconnected", provider, status)
		}
		if auth != nil {
			t.Errorf("%s kept its sealed credential after the seat was deactivated", provider)
		}
	}
	// The cascade is the departing seat's, not the company's.
	status, auth := connectionState(t, e, e.Rep2, "gmail")
	if status != "connected" || auth == nil {
		t.Fatalf("a colleague's connection was withdrawn too: status=%q auth=%v", status, auth != nil)
	}
}

// The second lock, and the one the operator actually feels: a departed seat's
// mailbox is not polled. Asserted through the poller's own selector rather
// than through the row, because the row being right is only half of it.
func TestADeactivatedSeatsMailboxIsNotDue(t *testing.T) {
	e := integration.Setup(t)
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))

	if !isDue(t, e, e.Rep1, "gmail") {
		t.Fatal("a live seat's connection is not due — the fixture proves nothing")
	}
	deactivate(t, e, e.Rep1)
	if isDue(t, e, e.Rep1, "gmail") {
		t.Fatal("a deactivated seat's mailbox is still due for capture")
	}
}

// Even with the row left connected — a cascade that missed it, or a row
// written before this existed — the poll refuses it on the seat's liveness
// alone. That is why the predicate is there as well as the cascade.
func TestThePollRefusesADepartedSeatEvenOnAConnectedRow(t *testing.T) {
	e := integration.Setup(t)
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))

	// Deactivated directly, so the cascade never ran and the row stays as it
	// was: exactly the state this predicate exists to catch.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("deactivating the seat directly: %v", err)
	}

	if status, _ := connectionState(t, e, e.Rep1, "gmail"); status != "connected" {
		t.Fatalf("fixture: the row should still read connected, got %q", status)
	}
	if isDue(t, e, e.Rep1, "gmail") {
		t.Fatal("a connected row belonging to a departed seat is still due")
	}
}

// A departure that stopped a mailbox says so in the trail, naming the status it
// was withdrawn FROM — so the record says which mailboxes it actually stopped,
// not merely that somebody left.
func TestWithdrawingOnDeactivationLeavesAnAuditRowPerConnection(t *testing.T) {
	e := integration.Setup(t)
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))
	seedCaptureConnection(t, e, e.Rep1, "imap", []byte(`{"password":"live"}`))

	deactivate(t, e, e.Rep1)

	rows := e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'capture_connection' AND action = 'archive'
		   AND after->>'status' = 'disconnected'
		   AND before->>'status' = 'connected'`)
	if rows != 2 {
		t.Fatalf("audit rows = %d, want one per withdrawn connection", rows)
	}
}

func deactivate(t *testing.T, e *integration.Env, seat ids.UUID) {
	t.Helper()
	admin := identity.Identity{
		UserID:      ids.From[ids.UserKind](e.AdminUser),
		WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType:    "full",
		Roles:       []string{"admin"},
		Permissions: integration.AdminPerms,
	}
	if err := identity.NewService(e.Pool).DeactivateUser(e.Admin(), admin,
		identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](seat)}); err != nil {
		t.Fatalf("deactivating the seat: %v", err)
	}
}

func seedCaptureConnection(t *testing.T, e *integration.Env, owner ids.UUID, provider string, auth []byte) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, auth)
			VALUES ($1, $2, 'connected', $3)`, provider, owner, auth)
		return err
	}); err != nil {
		t.Fatalf("seeding the %s connection: %v", provider, err)
	}
}

func connectionState(t *testing.T, e *integration.Env, owner ids.UUID, provider string) (string, []byte) {
	t.Helper()
	var status string
	var auth []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status, auth FROM capture_connection WHERE user_id = $1 AND provider = $2`,
			owner, provider).Scan(&status, &auth)
	}); err != nil {
		t.Fatalf("reading the %s connection: %v", provider, err)
	}
	return status, auth
}

// isDue asks the poller's own selector, which is the thing that decides
// whether a mailbox gets read.
func isDue(t *testing.T, e *integration.Env, owner ids.UUID, provider string) bool {
	t.Helper()
	due, err := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, nil).DueConnections(context.Background(), provider)
	if err != nil {
		t.Fatalf("DueConnections: %v", err)
	}
	var want ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM capture_connection WHERE user_id = $1 AND provider = $2`,
			owner, provider).Scan(&want)
	}); err != nil {
		t.Fatalf("finding the connection: %v", err)
	}
	for _, d := range due {
		if d.ID == want {
			return true
		}
	}
	return false
}
