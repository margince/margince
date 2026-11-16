// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The stored watch handle across a reconnect, over a real migrated Postgres.
//
// capture_connection.watch_ref names a push subscription in the mailbox the row
// belongs to. A grant for another account adds a second row and never re-points
// this one, so the handle survives every grant that lands on its row.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// renewalConnector answers for one fixed mailbox. Registered under its own
// provider name so it never disturbs the gmail row the rest of the package's
// fixtures use.
type renewalConnector struct {
	label string
	// duringRenewal runs inside RenewWatch, between the renewal's read of the
	// row and its write. It is a pointer because the registry refuses a second
	// registration under one name. The test arms it after the connect.
	duringRenewal *func()
}

func (renewalConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "graph", Version: "fixture"}
}

func (renewalConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("fixture-token"), nil
}

func (renewalConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (renewalConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}

func (renewalConnector) HealthCheck(context.Context, connector.Auth) error { return nil }

func (c renewalConnector) AccountLabel(connector.Auth) (string, error) { return c.label, nil }

func (renewalConnector) Watch(context.Context, connector.Auth, string) (connector.WatchResult, error) {
	return connector.WatchResult{ExpiresAt: watchDeadline, Ref: "sub-registered-fresh"}, nil
}

// RenewWatch returns the handle it was given with watchDeadline, so a write
// that landed shows as that deadline on the row.
func (c renewalConnector) RenewWatch(
	_ context.Context, _ connector.Auth, _, ref string,
) (connector.WatchResult, error) {
	if c.duringRenewal != nil && *c.duringRenewal != nil {
		(*c.duringRenewal)()
	}
	return connector.WatchResult{ExpiresAt: watchDeadline, Ref: ref}, nil
}

// watchDeadline is any instant far enough out to be obviously not the one the
// fixture seeded.
var watchDeadline = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

var (
	_ connector.AccountLabeler = renewalConnector{}
	_ connector.Watcher        = renewalConnector{}
	_ connector.WatchRenewer   = renewalConnector{}
)

// storedWatch reads back the handle and deadline one connection row holds.
func storedWatch(ctx context.Context, t *testing.T, id ids.UUID) (ref *string, expires *time.Time) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if err := owner.QueryRow(ctx,
		`SELECT watch_ref, watch_expires_at FROM capture_connection WHERE id = $1`, id).
		Scan(&ref, &expires); err != nil {
		t.Fatalf("reading the watch columns back: %v", err)
	}
	return ref, expires
}

// putWatch writes the handle a live subscription would have left on the one
// graph row.
func putWatch(ctx context.Context, t *testing.T, id ids.UUID) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if _, err := owner.Exec(ctx, `
		UPDATE capture_connection
		SET watch_ref = 'sub-in-the-first-mailbox', watch_expires_at = now() + interval '2 days'
		WHERE id = $1`, id); err != nil {
		t.Fatalf("seeding the stored watch: %v", err)
	}
	ref, expires := storedWatch(ctx, t, id)
	if ref == nil || expires == nil {
		t.Fatal("precondition: the row should hold a handle and a deadline before the reconnect")
	}
}

// A reconnect of the same account keeps the handle, so a re-consent does not
// cost the mailbox its push registration.
func TestReconnectingTheSameAccountKeepsTheStoredWatchHandle(t *testing.T) {
	t.Parallel()
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	reg.Register(renewalConnector{label: "same@example.test"})

	id, err := reg.Connect(ctx, "graph", connector.Auth("token-1"))
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	putWatch(ctx, t, id)

	again, err := reg.Connect(ctx, "graph", connector.Auth("token-2"))
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if again != id {
		t.Fatalf("the reconnect landed on row %v, not on the mailbox's own row %v", again, id)
	}

	ref, expires := storedWatch(ctx, t, id)
	if ref == nil || *ref != "sub-in-the-first-mailbox" || expires == nil {
		t.Errorf("watch_ref = %v, watch_expires_at = %v — a re-consent over the SAME mailbox "+
			"dropped a live subscription, and the mailbox falls back to the poll until the next scan", ref, expires)
	}
}

// A renewal in flight when the mailbox is disconnected writes nothing, even when
// the member connects it again before the renewal returns. The renewal ran on
// the withdrawn credential. The row's status cannot show that; the generation
// can.
func TestARenewalThatRacedADisconnectWritesNothing(t *testing.T) {
	t.Parallel()
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	var hook func()
	reg.Register(renewalConnector{label: "first@example.test", duringRenewal: &hook})
	id, err := reg.Connect(ctx, "graph", connector.Auth("token-1"))
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	putWatch(ctx, t, id)
	_, seeded := storedWatch(ctx, t, id)

	// Armed only now, so the disconnect and reconnect commit between this
	// renewal's read of the row and its write, and not during the connect above.
	hook = func() {
		if err := reg.Disconnect(ctx, capture.ProviderMailbox("graph")); err != nil {
			t.Errorf("the disconnect inside the renewal: %v", err)
		}
		if _, err := reg.Connect(ctx, "graph", connector.Auth("token-2")); err != nil {
			t.Errorf("the reconnect inside the renewal: %v", err)
		}
	}
	if err := reg.RenewWatch(ctx, id, "topic"); err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}

	_, expires := storedWatch(ctx, t, id)
	if expires == nil || !expires.Equal(*seeded) {
		t.Errorf("watch_expires_at = %v, want the seeded %v — a renewal made under the withdrawn "+
			"credential wrote its deadline onto the row the new grant now holds", expires, *seeded)
	}
}
