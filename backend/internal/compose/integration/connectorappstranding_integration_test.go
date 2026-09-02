// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Changing a connector app's CLIENT ID is the one edit that can strand
// mailboxes: a refresh token belongs to the client that issued it, so the
// connections authorized against the old id stop syncing the moment a new one
// lands, and nothing in the resulting error says why. These are the cases that
// decide which connections count as still holding one.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// A CLIENT ID CANNOT BE REPLACED WHILE MAILBOXES ARE CONNECTED UNDER IT.
//
// A refresh token belongs to the client that issued it. Swapping the id makes
// every stored token unrefreshable, and the vendor answers `invalid_client` — so
// the mailboxes stop syncing one by one, at whatever hour their next refresh
// falls, and from the inside it looks like every mailbox revoking access at
// once.
//
// The SECRET stays rotatable against the same id, which is the whole point of
// being able to rotate one.
func TestReplacingTheClientIDIsRefusedWhileConnectionsHoldTokensFromTheOldOne(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-first", ""); err != nil {
		t.Fatalf("storing: %v", err)
	}

	// A ROTATION FIRST, before any connection exists, so the refusal below is
	// about the id and not about the write path being closed.
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-second", ""); err != nil {
		t.Fatalf("rotating the secret with no connections: %v", err)
	}

	seedGmailConnection(ctx, t, e)

	// The secret still rotates with a mailbox connected — same client, same
	// tokens.
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-third", ""); err != nil {
		t.Fatalf("rotating the secret with a connection: %v", err)
	}

	const otherClientID = "9999999999-zyxwvutsrqponmlk.apps.googleusercontent.com"
	err := store.Set(ctx, capture.AppProviderGoogle, otherClientID, "GOCSPX-fourth", "")
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("replacing the client id answered %v, want a refusal — every connected mailbox holds a "+
			"refresh token the new client cannot use", err)
	}
	if !strings.Contains(invalid.Reason, "refresh token") {
		t.Errorf("the refusal reads %q, which does not tell the operator why", invalid.Reason)
	}

	// And the stored app is untouched: a refused write that half-committed
	// would be worse than the change it refused.
	app, _, err := store.Credentials(ctx, capture.AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving after the refusal: %v", err)
	}
	if app.ClientID != testClientID {
		t.Errorf("client id = %q after a refused change, want the one that was there", app.ClientID)
	}
	if app.ClientSecretRef != "GOCSPX-third" {
		t.Errorf("secret = %q after a refused change, want the one that was there", app.ClientSecretRef)
	}

	// A DISCONNECTED mailbox strands nobody, so the change goes through once
	// the operator has done what the refusal asked.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET status = 'disconnected' WHERE provider = 'gmail'`)
		return err
	}); err != nil {
		t.Fatalf("disconnecting: %v", err)
	}
	if err := store.Set(ctx, capture.AppProviderGoogle, otherClientID, "GOCSPX-fifth", ""); err != nil {
		t.Fatalf("replacing the client id with nothing connected: %v — the refusal has no way out", err)
	}
}

// seedGmailConnection writes one connected gmail mailbox. Written directly
// because the subject is the ROW the app change would strand, not how a consent
// callback puts it there.
// AN ERRORED CONNECTION IS STILL STRANDED BY A CHANGED CLIENT ID.
//
// A refresh token belongs to the client that issued it, and 'error' is a
// degraded connection rather than a dead one: it keeps that token and the sweep
// keeps trying it. Reading only 'connected' as live would wave the change
// through for precisely the installation most likely to be attempting it — one
// whose mailboxes have started failing — and leave them chasing the same
// misleading error afterwards, now with no way back.
func TestAnErroredConnectionStillRefusesAChangedClientID(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.captureAdmin()
	store := googleAppStore(t, e)

	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-first", ""); err != nil {
		t.Fatalf("storing the app: %v", err)
	}
	seedGmailConnection(ctx, t, e)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE capture_connection SET status = 'error' WHERE provider = 'gmail'`)
		return err
	}); err != nil {
		t.Fatalf("degrading the connection: %v", err)
	}

	err := store.Set(ctx, capture.AppProviderGoogle, "9999-zzzz.apps.googleusercontent.com", "GOCSPX-second", "")
	if err == nil {
		t.Fatal("a changed client id was accepted over an errored connection, stranding its refresh token")
	}
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("refused with %v, want a settings.InvalidValue the operator can read", err)
	}

	// AND ROTATION STILL WORKS. The guard is about the id, not the secret: an
	// operator rotating a leaked secret sends both fields, and refusing that
	// would leave them unable to respond to the very incident that needs it.
	if err := store.Set(ctx, capture.AppProviderGoogle, testClientID, "GOCSPX-rotated", ""); err != nil {
		t.Fatalf("rotating the secret under the same id was refused: %v", err)
	}
}

func seedGmailConnection(ctx context.Context, t *testing.T, e *SearchEnv) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (provider, user_id, scopes, status, auth)
			VALUES ('gmail', $1, '{}', 'connected', $2)`,
			e.Rep1, []byte(`{"refresh_token":"r","granted":[]}`))
		return err
	}); err != nil {
		t.Fatalf("seeding the gmail connection: %v", err)
	}
}
