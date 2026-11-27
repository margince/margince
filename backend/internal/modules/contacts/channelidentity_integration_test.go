// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The identity race two simultaneous first messages create. Both resolvers
// are released on one barrier and both attempt the bind; Postgres blocks the
// loser on the speculative-insert lock until the winner commits, so the
// DATABASE is the synchronizer — no sleep, no polling, no timing guess.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// resolveOrCreateContactForIdentity is the channel-aware ensure contract in
// miniature: resolve through the ladder, and when nothing resolves, create the
// contact and its identity satellite SPECULATIVELY inside a savepoint. A
// resolver that loses the bind adopts the winner's contact and rolls its own
// speculative row back, which is the only way one human ends up on one record
// — a created contact cannot be un-created any other way.
func (e *dedupeEnv) resolveOrCreateContactForIdentity(ctx context.Context, name string, ci connector.ChannelIdentity) (ids.ContactID, error) {
	var resolved ids.ContactID
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		r, err := DedupeContact(ctx, tx, ContactCandidate{
			FullName: name, ChannelIdentities: []connector.ChannelIdentity{ci},
		})
		if err != nil {
			return err
		}
		if r.Decision == DecisionExactCollision {
			resolved = r.ContactID
			return nil
		}

		speculative, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		candidate := ids.New[ids.ContactKind]()
		if _, err := speculative.Exec(ctx, `
			INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, $2, $3, $4)`,
			candidate, name, ci.Provider, "connector:"+ci.Provider); err != nil {
			return err
		}
		owner, err := ResolveOrCreateChannelIdentity(ctx, speculative, candidate, ci)
		if err != nil {
			return err
		}
		resolved = owner
		if owner != candidate {
			return speculative.Rollback(ctx)
		}
		return speculative.Commit(ctx)
	})
	return resolved, err
}

// The adopt branch, proved without depending on goroutine scheduling: an
// identity already bound to someone is never re-pointed, and the offered
// contact is told whose conversation it actually is.
func TestChannelIdentityBindAdoptsTheIncumbentOwner(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.seedContact(ctx, t, "Incumbent Owner", []string{"incumbent@adopt.test"}, nil)
	latecomer := e.seedContact(ctx, t, "Latecomer", []string{"late@adopt.test"}, nil)
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770501"}
	e.bindIdentity(ctx, t, incumbent, ci)

	if adopted := e.bindIdentity(ctx, t, latecomer, ci); adopted != incumbent {
		t.Fatalf("bind returned %s, want the incumbent %s", adopted, incumbent)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE contact_id = $1`, latecomer); n != 0 {
		t.Fatalf("the latecomer gained %d identity rows; a bound identity is never re-pointed by a bind", n)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1 AND contact_id = $2 AND archived_at IS NULL`,
		ci.ChannelUserID, incumbent); n != 1 {
		t.Fatalf("the incumbent holds %d live bindings for %s, want 1", n, ci.ChannelUserID)
	}
}

func TestTwoConcurrentResolutionsConvergeOnOneContact(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	const name = "Concurrent First Message"
	ci := connector.ChannelIdentity{Provider: telegramProvider, ChannelUserID: "770401", Username: "racer"}

	barrier := make(chan struct{})
	resolved := make([]ids.ContactID, 2)
	failures := make([]error, 2)
	var running sync.WaitGroup
	running.Add(len(resolved))
	for i := range resolved {
		go func(slot int) {
			defer running.Done()
			<-barrier
			resolved[slot], failures[slot] = e.resolveOrCreateContactForIdentity(ctx, name, ci)
		}(i)
	}
	close(barrier)
	running.Wait()

	for slot, err := range failures {
		if err != nil {
			t.Fatalf("resolver %d: %v", slot, err)
		}
	}
	if resolved[0] != resolved[1] {
		t.Fatalf("resolvers converged on %s and %s; two first messages from one Telegram user must land on one contact",
			resolved[0], resolved[1])
	}

	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact WHERE full_name = $1 AND archived_at IS NULL`, name); n != 1 {
		t.Fatalf("%d contact rows, want exactly 1 — the loser's speculative contact must leave no trace", n)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1 AND archived_at IS NULL`,
		ci.ChannelUserID); n != 1 {
		t.Fatalf("%d channel identity rows, want exactly 1", n)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE contact_id = $1 AND archived_at IS NULL`,
		resolved[0]); n != 1 {
		t.Fatalf("the surviving contact carries %d identity rows, want 1", n)
	}
}

// countInWorkspace runs one count under the workspace GUC, which is what the
// caller's own query predicates on to reach this test's tenant alone.
func (e *dedupeEnv) countInWorkspace(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return n
}

// The insert writes the account scope, and the scoped unique index arbitrates it.
// Until the older (provider, channel_user_id) index is dropped, one user id binds
// once across scopes, so only a collision in the same scope is asserted.
func TestAnIdentityIsBoundOncePerAccountScope(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	first := e.seedContact(ctx, t, "First Scope", []string{"first@scope.test"}, nil)
	other := e.seedContact(ctx, t, "Same Scope", []string{"same@scope.test"}, nil)
	const user = "770601"
	inFirst := connector.ChannelIdentity{Provider: telegramProvider, AccountScope: "880001", ChannelUserID: user}

	if bound := e.bindIdentity(ctx, t, first, inFirst); bound != first {
		t.Fatalf("the first bind returned %s, want %s", bound, first)
	}
	if bound := e.bindIdentity(ctx, t, other, inFirst); bound != first {
		t.Fatalf("a second bind in the same scope returned %s, want the incumbent %s", bound, first)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1 AND account_scope = $2 AND archived_at IS NULL`,
		user, inFirst.AccountScope); n != 1 {
		t.Fatalf("%d live bindings in the scope, want 1: the insert must write account_scope", n)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM pg_indexes WHERE indexname = 'uq_contact_channel_identity_scoped'`); n != 1 {
		t.Fatalf("the scoped unique index is missing")
	}
	resolved, err := e.resolveOrCreateContactForIdentity(ctx, "Resolver", inFirst)
	if err != nil || resolved != first {
		t.Fatalf("resolving the user in the scope = %s, %v, want %s", resolved, err, first)
	}
}

// The older (provider, channel_user_id) index still refuses a second scope for one
// user id. The bind reports that as ErrChannelIdentityInOtherScope and neither
// raises a unique violation nor hands back the other scope's contact.
func TestAnIdentityBoundInAnotherScopeIsRefusedAsAConflict(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	first := e.seedContact(ctx, t, "Scope Holder", []string{"holder@scope.test"}, nil)
	other := e.seedContact(ctx, t, "Scope Visitor", []string{"visitor@scope.test"}, nil)
	const user = "770602"
	if bound := e.bindIdentity(ctx, t, first, connector.ChannelIdentity{Provider: telegramProvider, AccountScope: "880002", ChannelUserID: user}); bound != first {
		t.Fatalf("the first bind returned %s, want %s", bound, first)
	}

	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := ResolveOrCreateChannelIdentity(ctx, tx, other,
			connector.ChannelIdentity{Provider: telegramProvider, AccountScope: "880003", ChannelUserID: user})
		return err
	})
	if !errors.Is(err, ErrChannelIdentityInOtherScope) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("binding the user in a second scope = %v, want ErrChannelIdentityInOtherScope as a conflict", err)
	}
	if n := e.countInWorkspace(ctx, t,
		`SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1 AND archived_at IS NULL`, user); n != 1 {
		t.Fatalf("%d live bindings, want the first scope's alone", n)
	}
}
