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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rebindConnector answers with whichever mailbox its pointer names, so one test
// can connect the same provider twice against two different accounts — which is
// all a rebind is. Registered under its own provider name so it never disturbs
// the gmail row the rest of the package's fixtures use.
type rebindConnector struct {
	label *string
	// duringRenewal runs inside RenewWatch, which is where a rebind that races
	// an in-flight renewal has to happen for the race to be the one under test.
	// A POINTER because the registry refuses a second registration under the
	// same name, so the hook has to be installable after the connection the
	// renewal is about already exists.
	duringRenewal *func()
}

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

func (rebindConnector) Watch(context.Context, connector.Auth, string) (connector.WatchResult, error) {
	return connector.WatchResult{ExpiresAt: watchDeadline, Ref: "sub-registered-fresh"}, nil
}

// RenewWatch returns the handle it was given, which is what makes the stale
// write visible: whatever the row held when the renewal started is what this
// tries to put back.
func (c rebindConnector) RenewWatch(
	_ context.Context, _ connector.Auth, _, ref string,
) (connector.WatchResult, error) {
	if c.duringRenewal != nil && *c.duringRenewal != nil {
		(*c.duringRenewal)()
	}
	return connector.WatchResult{ExpiresAt: watchDeadline, Ref: ref}, nil
}

// watchDeadline is any instant far enough out to be obviously not the one the
// fixture seeded; the tests read the handle, not the deadline.
var watchDeadline = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

var (
	_ connector.AccountLabeler = rebindConnector{}
	_ connector.Watcher        = rebindConnector{}
	_ connector.WatchRenewer   = rebindConnector{}
)

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

// connectionID reads back the id of the graph row, which is what
// Registry.RenewWatch is addressed by.
func connectionID(ctx context.Context, t *testing.T) ids.UUID {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var id ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT id FROM capture_connection WHERE provider = 'graph'`).Scan(&id); err != nil {
		t.Fatalf("reading the connection id: %v", err)
	}
	return id
}

// A RENEWAL THAT WAS IN FLIGHT WHEN THE REBIND LANDED WRITES NOTHING.
//
// Clearing the handle in the connect upsert is not enough on its own: the
// connector call is a round trip, and a renewal that read the old handle before
// the rebind committed would put it straight back afterwards — restoring a
// subscription in a mailbox this row no longer points at, which is the failure
// the clearing exists to prevent, arriving a moment later.
func TestARenewalThatRacedARebindDoesNotRestoreTheOldHandle(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	label := "first@example.test"
	rebind := func() {
		label = "second@example.test"
		if _, err := reg.Connect(ctx, "graph", connector.Auth("token-2")); err != nil {
			t.Errorf("the rebind inside the renewal: %v", err)
		}
	}
	var hook func()
	reg.Register(rebindConnector{label: &label, duringRenewal: &hook})
	if _, err := reg.Connect(ctx, "graph", connector.Auth("token-1")); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	putWatch(ctx, t)
	id := connectionID(ctx, t)

	// Armed only now, so the rebind commits between this renewal's read of the
	// handle and its write of the result, and not during the connect above.
	hook = rebind
	if err := reg.RenewWatch(ctx, id, "topic"); err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}

	ref, expires := storedWatch(ctx, t)
	if ref != nil {
		t.Errorf("watch_ref = %q — a renewal that started before the rebind put the previous "+
			"mailbox's subscription back, and the row now names one it does not own", *ref)
	}
	if expires != nil {
		t.Errorf("watch_expires_at = %v — the renewal scan will leave this row alone until a "+
			"deadline that belongs to another mailbox passes", *expires)
	}
}
