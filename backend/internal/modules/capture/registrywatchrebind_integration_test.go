// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A rebind and the stored watch handle, over a real migrated Postgres.
//
// capture_connection.watch_ref names a push subscription in the mailbox it was
// registered against. Pointing the row at a different mailbox leaves that
// subscription in the old one, so the handle stops describing this connection
// the instant the account changes. The reset belongs in the connect upsert,
// beside the sync cursor and the sharing posture that are cleared for the same
// reason, and it needs the real statement to prove.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rebindConnector answers with whichever mailbox its pointer names, so one test
// can connect the same provider twice against two different accounts — which is
// all a rebind is. Registered under its own provider name so it never disturbs
// the gmail row the rest of the package's fixtures use.
type rebindConnector struct{ label *string }

func (rebindConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "graph", Version: "fixture"}
}

func (rebindConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("fixture-token"), nil
}

func (rebindConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (rebindConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (rebindConnector) HealthCheck(context.Context, connector.Auth) error { return nil }

func (c rebindConnector) AccountLabel(connector.Auth) (string, error) { return *c.label, nil }

var _ connector.AccountLabeler = rebindConnector{}

// storedWatch reads back the handle and deadline the graph row holds.
func storedWatch(ctx context.Context, t *testing.T) (ref *string, expires *time.Time) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if err := owner.QueryRow(ctx,
		`SELECT watch_ref, watch_expires_at FROM capture_connection WHERE provider = 'graph'`).
		Scan(&ref, &expires); err != nil {
		t.Fatalf("reading the watch columns back: %v", err)
	}
	return ref, expires
}

// putWatch writes the handle a live subscription would have left, so the
// connect that follows has something to clear.
func putWatch(ctx context.Context, t *testing.T) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if _, err := owner.Exec(ctx, `
		UPDATE capture_connection
		SET watch_ref = 'sub-in-the-first-mailbox', watch_expires_at = now() + interval '2 days'
		WHERE provider = 'graph'`); err != nil {
		t.Fatalf("seeding the stored watch: %v", err)
	}
	ref, expires := storedWatch(ctx, t)
	if ref == nil || expires == nil {
		t.Fatal("precondition: the row should hold a handle and a deadline before the reconnect")
	}
}

// Kept, the handle would send the next renewal to extend the PREVIOUS mailbox's
// subscription — which the same app registration is entitled to do, so it would
// succeed — leaving the new mailbox with no push at all and nothing failing to
// say so. The deadline goes with it: a deadline for a subscription this row no
// longer owns keeps the renewal scan away until it lapses.
func TestRebindingToAnotherAccountDropsTheStoredWatchHandle(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	label := "first@example.test"
	reg.Register(rebindConnector{label: &label})

	if _, err := reg.Connect(ctx, "graph", connector.Auth("token-1")); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	putWatch(ctx, t)

	label = "second@example.test"
	if _, err := reg.Connect(ctx, "graph", connector.Auth("token-2")); err != nil {
		t.Fatalf("rebinding connect: %v", err)
	}

	ref, expires := storedWatch(ctx, t)
	if ref != nil {
		t.Errorf("watch_ref = %q after a rebind — the next renewal would extend a subscription "+
			"belonging to the mailbox this row no longer points at", *ref)
	}
	if expires != nil {
		t.Errorf("watch_expires_at = %v after a rebind — the renewal scan selects on this, so the "+
			"new mailbox waits for another account's deadline before anything registers for it", *expires)
	}
}

// AND A RECONNECT OF THE SAME ACCOUNT keeps it, which is what stops a
// re-consent costing every mailbox its push registration.
func TestReconnectingTheSameAccountKeepsTheStoredWatchHandle(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	label := "same@example.test"
	reg.Register(rebindConnector{label: &label})

	if _, err := reg.Connect(ctx, "graph", connector.Auth("token-1")); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	putWatch(ctx, t)

	if _, err := reg.Connect(ctx, "graph", connector.Auth("token-2")); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	ref, expires := storedWatch(ctx, t)
	if ref == nil || *ref != "sub-in-the-first-mailbox" || expires == nil {
		t.Errorf("watch_ref = %v, watch_expires_at = %v — a re-consent over the SAME mailbox "+
			"dropped a live subscription, and the mailbox falls back to the poll until the next scan", ref, expires)
	}
}
