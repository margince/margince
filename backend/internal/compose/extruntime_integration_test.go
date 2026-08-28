// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the per-call Runtime does once it is live, over real migrated
// Postgres: the callback runs inside ONE transaction carrying the invoking
// workspace, and the secret namespace it hands out belongs to the invoking
// unit and no other. Neither is checkable without a database.
//
// Everything here rides the APP pool (integration.Setup's Env.Pool, off
// MARGINCE_TEST_APP_DSN), which is the role a unit's SQL actually runs as.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// extRuntimeEnv is the fixture: the shared migrated database, plus an
// in-memory custodian, plus the bound process-wide runtime dependencies a
// role's boot would have bound.
type extRuntimeEnv struct {
	*integration.Env
	vault keyvault.Vault
}

func setupExtRuntime(t *testing.T) *extRuntimeEnv {
	t.Helper()
	e := integration.Setup(t)
	vault := keyvault.NewMemory()
	bindRuntimeForTest(t, e.Pool, vault)
	return &extRuntimeEnv{Env: e, vault: vault}
}

// callCtx is what a governed tool call arrives with: a bound workspace, an
// actor, and a correlation id — the secret store writes a system_log row on
// every operation and needs all three.
func (e *extRuntimeEnv) callCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "system:extruntime-test",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// runtime mints a Runtime the way the tool adapter does for an invocation
// arriving in the fixture workspace, and hands back that invocation's context
// alongside it. Keeping the two together is the point: the Runtime now
// derives its tenant from the context it was MINTED with, so a test that
// built one and then invented an unrelated context would be testing a shape
// the core never produces.
func (e *extRuntimeEnv) runtime(unit string) (*callRuntime, context.Context) {
	ctx := e.callCtx(e.WS)
	return runtimeFor(ctx, unit, "1.0.0", "tool/probe", extensionRuntimeBinding{pool: e.Pool, vault: e.vault}), ctx
}

// TestRuntimeTxOpensOneTransactionOnTheInvokingWorkspace: the callback is
// handed the invocation's tenant and a single transaction. Two statements
// reporting the same txid is what "one transaction" means from inside it —
// a helper that opened a connection per statement would report two.
func TestRuntimeTxOpensOneTransactionOnTheInvokingWorkspace(t *testing.T) {
	e := setupExtRuntime(t)
	rt, ctx := e.runtime("alpha")

	var scoped ids.UUID
	var first, second uint64
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			return errors.New("the callback ran with no workspace on its context")
		}
		scoped = ws
		if err := tx.QueryRow(ctx, `SELECT txid_current()`).Scan(&first); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT txid_current()`).Scan(&second)
	}); err != nil {
		t.Fatal(err)
	}
	if scoped != e.WS {
		t.Fatalf("the callback ran on workspace %q, want the invoking %q", scoped, e.WS)
	}
	if first != second {
		t.Fatalf("the callback's two statements ran in transactions %d and %d, want one", first, second)
	}
}

// TestRuntimeTxRefusesACallWithNoWorkspace: a Runtime is minted per
// invocation, and an invocation with no tenant bound has no workspace to pin
// to. Opening an unpinned transaction would hand a unit's SQL whatever the
// deny-on-unset policies happen to allow, so the seam refuses instead.
func TestRuntimeTxRefusesACallWithNoWorkspace(t *testing.T) {
	e := setupExtRuntime(t)
	// Minted from an INVOCATION with no tenant: that, and not the context the
	// handler passes, is where the workspace now comes from.
	rt := runtimeFor(context.Background(), "alpha", "1.0.0", "tool/probe", extensionRuntimeBinding{pool: e.Pool, vault: e.vault})

	ran := false
	err := rt.Tx(context.Background(), func(context.Context, extension.Tx) error {
		ran = true
		return nil
	})
	if err == nil {
		t.Fatal("an unpinned Tx opened anyway")
	}
	if ran {
		t.Fatal("the callback ran inside a transaction bound to no tenant")
	}
}

// TestRuntimeTxCommitsAndRollsBack walks the seam's own contract: the three
// verbs work, fn returning nil commits, fn returning an error rolls back.
//
// It runs against `app_user` — a CORE table, and one holding people's names —
// because the extension's own ext_* tables arrive with the demo unit and this
// seam has to be correct before there is one.
//
// That it SUCCEEDS is the honest demonstration, and the honest statement is
// blunter than the one this comment used to make: for a core table there is no
// containment wall at all. Core has carried no row-level security since 0217,
// so the tenant pin binds a GUC that only the extension tables' policies read,
// and this unit's SQL carries no workspace predicate of its own. runtime.go
// once claimed a per-unit database role; none exists, and #628 tracks building
// one. When it does, this test is expected to need a table the unit actually
// owns — that it currently does not is the point being recorded here rather
// than hidden.
//
// It used to write workspace.slug, until ADR-0091 retired that column. The
// substitute is deliberately not another workspace column: the row is identity
// and lifecycle now, so a unit writing it would be writing a timestamp or a
// key rather than anything a reader would recognise as damage. A seat's
// display name is legible as exactly what the missing wall would let through.
func TestRuntimeTxCommitsAndRollsBack(t *testing.T) {
	e := setupExtRuntime(t)
	rt, ctx := e.runtime("alpha")

	// Commit: rename a fixture seat, then read it back in a SECOND
	// transaction, so what is asserted is durability rather than the write's
	// own snapshot.
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		n, err := tx.Exec(ctx, `UPDATE app_user SET display_name = $1 WHERE id = $2`, "committed", e.Rep1)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("Exec reported %d rows affected, want 1", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.seatName(ctx, t, rt); got != "committed" {
		t.Fatalf("after a committing Tx the row reads %q, want committed", got)
	}

	// Rollback: the same write, abandoned. The error the callback returns is
	// the one the caller sees — not a wrapped commit failure.
	sentinel := errors.New("the handler changed its mind")
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE app_user SET display_name = $1 WHERE id = $2`, "rolled-back", e.Rep1); err != nil {
			return err
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("Tx returned %v, want the callback's own error", err)
	}
	if got := e.seatName(ctx, t, rt); got != "committed" {
		t.Fatalf("after a rolled-back Tx the row reads %q, want the committed value to have survived", got)
	}
}

// seatName reads the fixture seat back through the seam, exercising Query/Rows
// on the way — the cursor idiom the published Rows documents, iterated to
// exhaustion and checked with Err. What these tests need is any column the unit
// can write through the seam, not this column in particular.
func (e *extRuntimeEnv) seatName(ctx context.Context, t *testing.T, rt *callRuntime) string {
	t.Helper()
	var name string
	seen := 0
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		rows, err := tx.Query(ctx, `SELECT display_name FROM app_user WHERE id = $1`, e.Rep1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := rows.Scan(&name); err != nil {
				return err
			}
			seen++
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	// The row was VISIBLE, which is the only thing a primary-key lookup can
	// report: zero would mean the seam hid it, not that a second one appeared.
	// Nothing in the database filters by tenant, so this says nothing about
	// which workspace the callback carries — that is
	// TestRuntimeTxOpensOneTransactionOnTheInvokingWorkspace's subject.
	if seen != 1 {
		t.Fatalf("the read saw %d rows for the seeded seat's primary key, want it visible through the seam", seen)
	}
	return name
}

// TestRuntimeQueryRowReportsAnEmptyMatchAsErrNoRows: the published Row.Scan
// promises the extension's own sentinel, not pgx's — a unit that matched on
// pgx.ErrNoRows would be binding a driver the surface never published.
func TestRuntimeQueryRowReportsAnEmptyMatchAsErrNoRows(t *testing.T) {
	e := setupExtRuntime(t)
	rt, ctx := e.runtime("alpha")

	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, ids.NewV7()).Scan(&name)
		if !errors.Is(err, extension.ErrNoRows) {
			t.Errorf("Scan on an empty match = %v, want extension.ErrNoRows", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRuntimeSecretsReachTheUserScopedNamespace: the port's other three
// verbs, which are a SEPARATE namespace rather than a variation on the first
// — a unit may hold an installation credential and a per-member one under
// one key name, and the workspace read must not answer with the member's.
func TestRuntimeSecretsReachTheUserScopedNamespace(t *testing.T) {
	e := setupExtRuntime(t)
	rt, ctx := e.runtime("alpha")
	user := extension.UserID(e.Rep1.String())

	if err := rt.Secrets().PutUser(ctx, user, "token", []byte("rep1's token")); err != nil {
		t.Fatal(err)
	}
	got, err := rt.Secrets().GetUser(ctx, user, "token")
	if err != nil || string(got) != "rep1's token" {
		t.Fatalf("GetUser = %q, %v; want the stored token", got, err)
	}
	// Same key, other namespace: absent, not the member's value.
	if _, err := rt.Secrets().Get(ctx, "token"); !errors.Is(err, extension.ErrSecretNotFound) {
		t.Fatalf("the workspace scope answered from the user scope: err=%v", err)
	}
	if err := rt.Secrets().DeleteUser(ctx, user, "token"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Secrets().GetUser(ctx, user, "token"); !errors.Is(err, extension.ErrSecretNotFound) {
		t.Fatalf("GetUser after DeleteUser = %v, want ErrSecretNotFound", err)
	}
}

// TestRuntimeTxSurfacesTheDatabasesOwnRefusal: the seam does not parse,
// rewrite or interpret a unit's SQL, so a statement the database refuses
// comes back as the database's own error and the transaction is abandoned.
// That matters because the containment wall IS the database's — the tenant
// pin and the policies that read it — so what it refuses must reach the unit
// unedited, with its SQLSTATE intact.
//
// The SQLSTATE is what is asserted, not merely that an error came back: a
// transaction that failed to open at all would also be non-nil, and would
// prove nothing about the statement ever reaching Postgres.
func TestRuntimeTxSurfacesTheDatabasesOwnRefusal(t *testing.T) {
	e := setupExtRuntime(t)
	rt, ctx := e.runtime("alpha")

	// 42P01 is undefined_table. The statement reached the server, the server
	// judged it, and its verdict is what the unit holds.
	const undefinedTable = "42P01"

	execErr := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO ext_no_such_table (id) VALUES (1)`)
		return err
	})
	if got := sqlState(execErr); got != undefinedTable {
		t.Fatalf("Exec against a missing table gave SQLSTATE %q (err=%v), want %s", got, execErr, undefinedTable)
	}
	queryErr := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Query(ctx, `SELECT * FROM ext_no_such_table`)
		return err
	})
	if got := sqlState(queryErr); got != undefinedTable {
		t.Fatalf("Query against a missing table gave SQLSTATE %q (err=%v), want %s", got, queryErr, undefinedTable)
	}
}

// sqlState digs the server's own error code out of whatever the seam wrapped
// it in, or returns "" when the error never came from Postgres at all.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// TestRuntimeTxIgnoresAWorkspaceTheHandlerSuppliesItself is the design's
// load-bearing claim, made structural. A handler holds a context and can
// build another; what it must not be able to do is make the transaction run
// somewhere else. The core re-derives the tenant from the invocation every
// time, so a context naming a different workspace is simply overwritten.
func TestRuntimeTxIgnoresAWorkspaceTheHandlerSuppliesItself(t *testing.T) {
	e := setupExtRuntime(t)
	rt, _ := e.runtime("alpha")

	// The handler's own context, pointed at a tenant this invocation has no
	// business in — exactly what a unit reaching for its neighbour's rows
	// would write.
	elsewhere := ids.NewV7()
	hostile := principal.WithWorkspaceID(e.callCtx(e.WS), elsewhere)

	var pinned ids.UUID
	if err := rt.Tx(hostile, func(ctx context.Context, _ extension.Tx) error {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			return errors.New("the callback ran with no workspace on its context")
		}
		pinned = ws
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if pinned == elsewhere {
		t.Fatalf("the handler re-pointed its own transaction at workspace %q", pinned)
	}
	if pinned != e.WS {
		t.Fatalf("the transaction ran on %q, want the invoking %q", pinned, e.WS)
	}

	// The same for the secret namespace, which resolves its tenant from the
	// context it is handed too: a hostile context must not move the namespace.
	if err := rt.Secrets().Put(hostile, "signing", []byte("alpha's key")); err != nil {
		t.Fatal(err)
	}
	// Read it back under the honest invocation context. If Put had honoured
	// the hostile tenant, this read would find nothing.
	honest, _ := e.runtime("alpha")
	if got, err := honest.Secrets().Get(e.callCtx(e.WS), "signing"); err != nil || string(got) != "alpha's key" {
		t.Fatalf("the secret landed in the workspace the handler named, not the invoking one: %q, %v", got, err)
	}
}

// TestRuntimeSecretsCannotReachAnotherUnitsNamespace is the wall itself: two
// units, one key name, two Runtimes the core built. Beta's Runtime is not
// beta's to point elsewhere — it closes over "beta" and every statement it
// issues carries it — so alpha's secret is not merely denied, it is absent.
func TestRuntimeSecretsCannotReachAnotherUnitsNamespace(t *testing.T) {
	e := setupExtRuntime(t)
	alpha, ctx := e.runtime("alpha")
	if err := alpha.Secrets().Put(ctx, "signing", []byte("alpha's key")); err != nil {
		t.Fatal(err)
	}
	if got, err := alpha.Secrets().Get(ctx, "signing"); err != nil || string(got) != "alpha's key" {
		t.Fatalf("alpha cannot read its own secret back: %q, %v", got, err)
	}

	beta, _ := e.runtime("beta")
	if _, err := beta.Secrets().Get(ctx, "signing"); !errors.Is(err, extension.ErrSecretNotFound) {
		t.Fatalf("beta's Runtime read across the namespace wall: err=%v", err)
	}
	// A delete is the destructive half of the same wall: beta must not be able
	// to revoke a credential alpha depends on.
	if err := beta.Secrets().Delete(ctx, "signing"); !errors.Is(err, extension.ErrSecretNotFound) {
		t.Fatalf("beta's Runtime deleted across the namespace wall: err=%v", err)
	}
	if got, err := alpha.Secrets().Get(ctx, "signing"); err != nil || string(got) != "alpha's key" {
		t.Fatalf("alpha's secret did not survive beta's delete: %q, %v", got, err)
	}
}

// TestNotesSigningKeyIsUnreachableFromASecondUnit is the same wall as the
// test above, driven with the REAL unit names the reference extension and its
// throwaway counterpart ship under.
//
// The generic alpha/beta case proves the mechanism. This one proves the CLAIM
// the demo makes to whoever is watching it: notes stores a signing key and
// signs with it, and fixtures/extensions/crm-nosy — a unit that declares the
// same workspace-scoped `signing` key, deliberately, so this is a question
// about a namespace and not about two units that picked different names — gets
// ErrSecretNotFound. gen-composition's
// TestTheNamespaceWallFixtureDeclaresTheSameKeyAsNotes keeps the two
// declarations agreeing, or this would pass for the wrong reason.
//
// The units are named as STRINGS rather than imported: the backend reaches an
// extension only through the generated composition (extensions_arch_test.go),
// and a unit's name is what the core scopes a Runtime by anyway.
func TestNotesSigningKeyIsUnreachableFromASecondUnit(t *testing.T) {
	e := setupExtRuntime(t)
	const key = "signing"
	material := []byte("the demo workspace's HMAC key")

	demo, ctx := e.runtime("notes")
	if err := demo.Secrets().Put(ctx, key, material); err != nil {
		t.Fatal(err)
	}

	nosy, _ := e.runtime("crm-nosy")
	if _, err := nosy.Secrets().Get(ctx, key); !errors.Is(err, extension.ErrSecretNotFound) {
		t.Fatalf("crm-nosy read notes's signing key: err=%v", err)
	}
	// Storing its OWN key under the same name must not disturb notes's, and
	// must not be readable as notes's: two namespaces, one key name.
	if err := nosy.Secrets().Put(ctx, key, []byte("crm-nosy's own key")); err != nil {
		t.Fatal(err)
	}

	// The demo can still sign, and signs with ITS key. That is the whole
	// capability the screen demonstrates, so it is what the wall must leave
	// intact.
	stored, err := demo.Secrets().Get(ctx, key)
	if err != nil || !bytes.Equal(stored, material) {
		t.Fatalf("notes's own key after crm-nosy wrote one under the same name: %q, %v", stored, err)
	}
	mine := hmac.New(sha256.New, stored)
	mine.Write([]byte("demo payload"))
	theirs := hmac.New(sha256.New, []byte("crm-nosy's own key"))
	theirs.Write([]byte("demo payload"))
	if hmac.Equal(mine.Sum(nil), theirs.Sum(nil)) {
		t.Fatal("the two units' signatures agree, so the key material did not stay separate")
	}
}
