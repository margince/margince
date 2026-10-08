// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A dead credential is removed, and a live one is not.
//
// Each table is its own case because "dead" is a different sentence for each:
// a revoked session, a used invite and a consumed setup token are all dead with
// their expiry still in the future, and a shared expires_at rule would keep
// every one of them.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyADeadCredentialIsReaped(t *testing.T) {
	_, pool := setupIdentityDB(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		table string
		// dead and live are the column lists and values that make one row of
		// each kind, written through the same INSERT so the only difference
		// between them is what makes one dead.
		cols       string
		dead, live string
	}{
		{
			name:  "a revoked session, its deadlines still ahead",
			table: "session",
			cols:  "user_id, token_hash, idle_expires_at, expires_at, revoked_at",
			dead:  "$1, 'reap-dead', now() + interval '1 day', now() + interval '1 day', now()",
			live:  "$1, 'reap-live', now() + interval '1 day', now() + interval '1 day', NULL",
		},
		{
			name:  "a session idle past its window, absolute deadline ahead",
			table: "session",
			cols:  "user_id, token_hash, idle_expires_at, expires_at",
			dead:  "$1, 'reap-idle-dead', now() - interval '1 minute', now() + interval '1 day'",
			live:  "$1, 'reap-idle-live', now() + interval '1 day', now() + interval '1 day'",
		},
		{
			name:  "an invite already used, expiry ahead",
			table: "auth_token",
			cols:  "user_id, purpose, token_hash, expires_at, used_at",
			dead:  "$1, 'invite', 'reap-dead', now() + interval '1 day', now()",
			live:  "$1, 'invite', 'reap-live', now() + interval '1 day', NULL",
		},
		{
			name:  "a consumed setup token, which carries no expiry at all",
			table: "setup_token",
			cols:  "token_hash, consumed_at",
			dead:  "'reap-dead', now()",
			live:  "'reap-live', NULL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer rollback(t, tx)

			user := seedReapUser(ctx, t, tx)
			insert := "INSERT INTO " + tc.table + " (" + tc.cols + ") VALUES "
			for _, values := range []string{tc.dead, tc.live} {
				// $1 is the owner, and not every credential has one: a setup
				// token belongs to the installation rather than to a member.
				var args []any
				if strings.Contains(values, "$1") {
					args = append(args, user)
				}
				if _, err := tx.Exec(ctx, insert+"("+values+")", args...); err != nil {
					t.Fatalf("seeding %s: %v", tc.table, err)
				}
			}
			if n := reapCount(ctx, t, tx, tc.table); n != 2 {
				t.Fatalf("seeded %d rows, want 2 — the assertions below would prove nothing", n)
			}

			if err := reapDeadCredentials(ctx, tx, tc.table); err != nil {
				t.Fatalf("reaping %s: %v", tc.table, err)
			}
			if n := reapCount(ctx, t, tx, tc.table); n != 1 {
				t.Errorf("%s holds %d rows after the reap, want the live one alone", tc.table, n)
			}
			var survivor string
			if err := tx.QueryRow(ctx,
				"SELECT token_hash FROM "+tc.table).Scan(&survivor); err != nil {
				t.Fatalf("reading the survivor: %v", err)
			}
			if survivor == "" || survivor[len(survivor)-4:] != "live" {
				t.Errorf("the row that survived is %q, which is the one the rule calls dead", survivor)
			}
		})
	}
}

// A table nobody wrote a rule for fails rather than sweeping nothing: a reaper
// that skips without saying so reads the same as one that found nothing.
func TestAnUnknownCredentialTableIsRefused(t *testing.T) {
	_, pool := setupIdentityDB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, tx)

	if err := reapDeadCredentials(ctx, tx, "passport"); err == nil {
		t.Error("a table with no dead-row rule was swept without complaint")
	}
}

func seedReapUser(ctx context.Context, t *testing.T, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name, status)
		 VALUES ('reap@example.invalid', 'Reaped', 'active') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seeding the owner: %v", err)
	}
	return id
}

func reapCount(ctx context.Context, t *testing.T, tx pgx.Tx, table string) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func rollback(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(context.Background()); err != nil {
		t.Errorf("rolling back: %v", err)
	}
}

// A reap that cannot run refuses the credential rather than minting it anyway.
//
// What this cannot separate is error handling from transaction semantics.
// Both statements share the caller's transaction, so a refused delete aborts it
// and the insert fails whether the writer returns the reap's error or drops it:
// swallowing the error with `_ =` keeps this test passing. The assertion is
// therefore about the outcome a caller sees, not about which line produced it.
//
// Saying so beats implying more. The reason the writers return the error is
// that a dropped one leaves the caller working against a transaction the
// database has already abandoned, and that belongs in the writer's own reading
// rather than in a claim this test cannot hold.
func TestAFailedReapRefusesTheCredential(t *testing.T) {
	owner, pool := setupIdentityDB(t)
	ctx := context.Background()
	// The owner connection, because creating a trigger needs rights the app
	// role does not have on the public schema.
	refuseSessionDeletes(ctx, t, owner)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, tx)
	user := seedReapUser(ctx, t, tx)
	// One dead row for the trigger to refuse. Without it the reap deletes
	// nothing, the trigger never fires, and this would pass on a writer that
	// swallowed the error.
	if _, err := tx.Exec(ctx,
		`INSERT INTO session (user_id, token_hash, idle_expires_at, expires_at, revoked_at)
		 VALUES ($1, 'refused-dead', now() + interval '1 day', now() + interval '1 day', now())`,
		user); err != nil {
		t.Fatalf("seeding the row the reap must remove: %v", err)
	}

	if err := insertSession(ctx, tx, ids.From[ids.UserKind](mustParseUUID(t, user)), "refused-live"); err == nil {
		t.Error("the session was minted though its reap could not run")
	}
}

// refuseSessionDeletes makes the one dead session undeletable, which is how a
// statement inside the writer's own transaction can be made to fail without
// touching what the writer inserts. Dropped on cleanup, and conditioned on the
// token this test writes so a crash leaves a trigger that fires for nothing.
func refuseSessionDeletes(ctx context.Context, t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
},
) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION refuse_session_deletes() RETURNS trigger AS $$
		 BEGIN RAISE EXCEPTION 'this session refuses deletion'; END $$ LANGUAGE plpgsql`,
		`CREATE TRIGGER trg_refuse_session_deletes BEFORE DELETE ON session
		 FOR EACH ROW WHEN (OLD.token_hash = 'refused-dead')
		 EXECUTE FUNCTION refuse_session_deletes()`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("installing the refusing trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DROP TRIGGER IF EXISTS trg_refuse_session_deletes ON session`,
			`DROP FUNCTION IF EXISTS refuse_session_deletes()`,
		} {
			if _, err := pool.Exec(context.Background(), stmt); err != nil {
				t.Errorf("dropping the refusing trigger: %v", err)
			}
		}
	})
}

func mustParseUUID(t *testing.T, raw string) ids.UUID {
	t.Helper()
	parsed, err := ids.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return parsed
}

// The same for the setup token, whose reap is reached through the service
// rather than a bare writer: a second call site, and the one credential here
// that belongs to the installation rather than to a member.
func TestAFailedSetupTokenReapRefusesTheToken(t *testing.T) {
	owner, pool := setupIdentityDB(t)
	ctx := context.Background()
	svc := NewService(pool)

	// A consumed token for the trigger to refuse, so the reap has work and the
	// refusal is reached. Without it the delete matches nothing and this would
	// pass over a reap that never ran.
	if _, err := pool.Exec(ctx,
		`INSERT INTO setup_token (token_hash, consumed_at) VALUES ('refused-setup', now())`); err != nil {
		t.Fatalf("seeding the row the reap must remove: %v", err)
	}
	refuseSetupTokenDeletes(ctx, t, owner)

	if _, err := svc.issueSetupToken(setupTokenActor(ctx), keepOutstanding); err == nil {
		t.Error("a setup token was issued though its reap could not run")
	}
}

func refuseSetupTokenDeletes(ctx context.Context, t *testing.T, owner interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
},
) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION refuse_setup_token_deletes() RETURNS trigger AS $$
		 BEGIN RAISE EXCEPTION 'this setup token refuses deletion'; END $$ LANGUAGE plpgsql`,
		`CREATE TRIGGER trg_refuse_setup_token_deletes BEFORE DELETE ON setup_token
		 FOR EACH ROW WHEN (OLD.token_hash = 'refused-setup')
		 EXECUTE FUNCTION refuse_setup_token_deletes()`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("installing the refusing trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DROP TRIGGER IF EXISTS trg_refuse_setup_token_deletes ON setup_token`,
			`DROP FUNCTION IF EXISTS refuse_setup_token_deletes()`,
		} {
			if _, err := owner.Exec(context.Background(), stmt); err != nil {
				t.Errorf("dropping the refusing trigger: %v", err)
			}
		}
	})
}
