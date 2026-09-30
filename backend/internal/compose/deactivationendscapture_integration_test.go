// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Deactivating a seat has to stop their mailbox being read. Everything else the
// cascade ends is a credential this product issued; a capture connection holds
// the PROVIDER's, and the poller that spends it never asked whose seat it was.

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
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
	if err := identity.NewService(e.Pool).DeactivateUser(e.Admin(), adminIdentity(e),
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

// The audit row says WHY, not just what: a withdrawal by departure and one by
// hand leave the same field images, and only the evidence tells them apart.
func TestTheWithdrawalAuditNamesTheDeactivation(t *testing.T) {
	e := integration.Setup(t)
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))

	deactivate(t, e, e.Rep1)

	reasons := e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'capture_connection' AND action = 'archive'
		   AND evidence->>'reason' IS NOT NULL AND evidence->>'reason' <> ''`)
	if reasons != 1 {
		t.Fatalf("audit rows naming a reason = %d, want the withdrawal to say why", reasons)
	}
}

// The destruction half, which runs after the withdrawal commits: the pointer
// to the vaulted secret is cleared, so nothing is left naming a blob that
// should no longer exist.
func TestReapingClearsTheCredentialPointerOfAWithdrawnConnection(t *testing.T) {
	e := integration.Setup(t)
	vault := keyvault.NewMemory()
	ref, err := vault.Put(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), []byte(`{"token":"secret"}`))
	if err != nil {
		t.Fatalf("sealing the credential: %v", err)
	}
	seedVaultedConnection(t, e, e.Rep1, string(ref))

	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, vault)
	// Withdrawn first, exactly as the deactivation transaction leaves it.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET status = 'disconnected', auth = NULL WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	if err := r.ReapWithdrawnCredentials(e.As(e.Rep1, nil, integration.AccountRepPerms),
		ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatalf("ReapWithdrawnCredentials: %v", err)
	}
	if got := credentialRef(t, e, e.Rep1); got != nil {
		t.Fatalf("credential_ref = %q, want it cleared once the secret is destroyed", *got)
	}
	// And the secret itself is gone, which is the point — the pointer being
	// clear would otherwise only mean nothing can find it.
	if _, err := vault.Get(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), ref); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("the vaulted credential survived the reap: %v", err)
	}
}

// A connection still LIVE keeps its credential, whatever else the seat has
// had withdrawn. Reaping is for what a withdrawal left behind, and a pass
// that reached a connected row would strand a secret it still needs.
func TestReapingLeavesALiveConnectionAlone(t *testing.T) {
	e := integration.Setup(t)
	seedVaultedConnection(t, e, e.Rep1, "ref-still-in-use")

	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, keyvault.NewMemory())
	if err := r.ReapWithdrawnCredentials(e.As(e.Rep1, nil, integration.AccountRepPerms),
		ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatalf("ReapWithdrawnCredentials: %v", err)
	}
	ref := credentialRef(t, e, e.Rep1)
	if ref == nil || *ref != "ref-still-in-use" {
		t.Fatalf("a connected connection lost its credential pointer: %v", ref)
	}
}

// Always gmail: what the vaulted path does is the same whichever mailbox
// holds the credential, and the provider only has to be one the reap can find.
func seedVaultedConnection(t *testing.T, e *integration.Env, owner ids.UUID, ref string) {
	const provider = "gmail"
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, credential_ref)
			VALUES ($1, $2, 'connected', $3)`, provider, owner, ref)
		return err
	}); err != nil {
		t.Fatalf("seeding the vaulted %s connection: %v", provider, err)
	}
}

func credentialRef(t *testing.T, e *integration.Env, owner ids.UUID) *string {
	const provider = "gmail"
	t.Helper()
	var ref *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT credential_ref FROM capture_connection WHERE user_id = $1 AND provider = $2`,
			owner, provider).Scan(&ref)
	}); err != nil {
		t.Fatalf("reading the credential ref: %v", err)
	}
	return ref
}

// The two halves wired together, which is the thing that would silently not
// work: a reaper nobody bound leaves the secret exactly where a reaper that
// was never written would.
func TestAWiredDeactivationDestroysTheVaultedCredential(t *testing.T) {
	e := integration.Setup(t)
	vault := keyvault.NewMemory()
	ref, err := vault.Put(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), []byte(`{"token":"secret"}`))
	if err != nil {
		t.Fatalf("sealing the credential: %v", err)
	}
	seedVaultedConnection(t, e, e.Rep1, string(ref))

	// Bound through the real seam: the port binds on the SERVICE, so the
	// handlers and the writer below are the same installation.
	svc := identity.NewService(e.Pool)
	registry := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, vault)
	identity.NewHandlers(svc).WithCaptureCredentialReaper(
		reapWithdrawnCredentials(registry, slog.New(slog.DiscardHandler)))

	if err := svc.DeactivateUser(e.Admin(), adminIdentity(e),
		identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep1)}); err != nil {
		t.Fatalf("deactivating the seat: %v", err)
	}

	if got := credentialRef(t, e, e.Rep1); got != nil {
		t.Errorf("credential_ref = %q, want it cleared by the wired reap", *got)
	}
	if _, err := vault.Get(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), ref); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("the departing seat's vaulted credential survived: %v", err)
	}
}

// A role that composed no capture registry binds nothing and says so by not
// panicking: deactivation still withdraws, and the secret waits for the sweep.
func TestInstallingTheReaperWithoutACaptureRegistryBindsNothing(t *testing.T) {
	e := integration.Setup(t)
	srv := Server{authHandlers: identity.NewHandlers(identity.NewService(e.Pool))}

	installCaptureCredentialReaper(&srv, slog.New(slog.DiscardHandler))

	// Deactivation still works, and still withdraws.
	seedCaptureConnection(t, e, e.Rep1, "gmail", []byte(`{"token":"live"}`))
	deactivate(t, e, e.Rep1)
	if status, _ := connectionState(t, e, e.Rep1, "gmail"); status != "disconnected" {
		t.Fatalf("status = %q, want the withdrawal to happen with no reaper wired", status)
	}
}

// A reap that cannot reach a vault is logged and swallowed rather than failing
// a departure that already committed.
func TestTheReaperAdapterDoesNotFailADepartureItCannotFinish(t *testing.T) {
	e := integration.Setup(t)
	seedVaultedConnection(t, e, e.Rep1, "ref-with-no-vault")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET status = 'disconnected' WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	// No vault on the registry: the reap reports a wiring fault, and the
	// adapter must absorb it.
	reap := reapWithdrawnCredentials(
		capture.NewRegistry(InstallationDB(e.Pool), nil, nil, nil), slog.New(slog.DiscardHandler))
	reap(e.As(e.Rep1, nil, integration.AccountRepPerms), ids.From[ids.UserKind](e.Rep1))

	// The pointer is deliberately still there: clearing it would leave the only
	// name for a secret nobody deleted.
	if got := credentialRef(t, e, e.Rep1); got == nil {
		t.Fatal("the credential pointer was cleared without the secret being destroyed")
	}
}

func adminIdentity(e *integration.Env) identity.Identity {
	return identity.Identity{
		UserID:      ids.From[ids.UserKind](e.AdminUser),
		WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType:    "full",
		Roles:       []string{"admin"},
		Permissions: integration.AdminPerms,
	}
}

// The reap runs on the post-commit cleanup budget, not the caller's. A client
// that hangs up the moment it has its answer must not be the reason a revoked
// credential stays decryptable — nor, worse, the reason a secret is destroyed
// while the row still names it.
func TestReapingSurvivesACallerThatHasAlreadyHungUp(t *testing.T) {
	e := integration.Setup(t)
	vault := keyvault.NewMemory()
	ref, err := vault.Put(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), []byte(`{"token":"secret"}`))
	if err != nil {
		t.Fatalf("sealing the credential: %v", err)
	}
	seedVaultedConnection(t, e, e.Rep1, string(ref))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET status = 'disconnected' WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	// Cancelled BEFORE the reap is asked to run, which is the worst version of
	// the request ending early.
	ctx, cancel := context.WithCancel(e.As(e.Rep1, nil, integration.AccountRepPerms))
	cancel()

	r := capture.NewRegistry(InstallationDB(e.Pool), nil, nil, vault)
	if err := r.ReapWithdrawnCredentials(ctx, ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatalf("ReapWithdrawnCredentials on a cancelled caller: %v", err)
	}
	if got := credentialRef(t, e, e.Rep1); got != nil {
		t.Errorf("credential_ref = %q, want it cleared despite the caller hanging up", *got)
	}
	if _, err := vault.Get(e.Admin(), ids.From[ids.WorkspaceKind](e.WS), ref); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("the credential survived a reap whose caller hung up: %v", err)
	}
}
