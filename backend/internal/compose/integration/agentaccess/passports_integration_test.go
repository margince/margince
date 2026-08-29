// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The passport list surface (GET /passports, feedback/13): metadata
// only, own rows for a regular user, workspace-wide for the admin role
// — the same authority split RevokePassport enforces. The HTTP-level
// token-never-re-disclosed assertion rides the e2e agent suite; this
// lane pins the service-layer scoping against the real migrated schema.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type passportsEnv struct {
	svc   *identity.Service
	WS    ids.UUID
	alice ids.UUID
	bob   ids.UUID
}

func setupPassports(t *testing.T) *passportsEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()

	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &passportsEnv{WS: ids.NewV7(), alice: ids.NewV7(), bob: ids.NewV7()}
	if _, err := owner.Exec(ctx,
		`INSERT INTO workspace (id) VALUES ($1)`, e.WS); err != nil {
		t.Fatal(err)
	}
	for i, user := range []ids.UUID{e.alice, e.bob} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'User')`, user, string(rune('a'+i))+"@passports.test"); err != nil {
			t.Fatal(err)
		}
	}

	var pool *pgxpool.Pool
	pool, err = testdb.OwnPool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e.svc = identity.NewService(pool)
	return e
}

func (e *passportsEnv) identityFor(user ids.UUID, roles []string) identity.Identity {
	return identity.Identity{UserID: ids.From[ids.UserKind](user), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS), Roles: roles}
}

func (e *passportsEnv) ctx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// A user lists exactly their own passports; the admin role sees the
// workspace's; the rows are metadata only.
func TestListPassportsScopesToOwnerUnlessAdmin(t *testing.T) {
	e := setupPassports(t)
	ctx := e.ctx()

	label := func(s string) *string { return &s }
	aliceIssued, err := e.svc.IssuePassport(ctx, e.identityFor(e.alice, []string{"rep"}),
		identity.IssuePassportInput{Label: label("alice claude"), Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("alice mint: %v", err)
	}
	if _, err := e.svc.IssuePassport(ctx, e.identityFor(e.bob, []string{"rep"}),
		identity.IssuePassportInput{Label: label("bob cursor"), Scopes: []string{"read", "draft"}}); err != nil {
		t.Fatalf("bob mint: %v", err)
	}

	aliceRows, err := e.svc.ListPassports(ctx, e.identityFor(e.alice, []string{"rep"}))
	if err != nil {
		t.Fatalf("alice list: %v", err)
	}
	if len(aliceRows) != 1 {
		t.Fatalf("alice sees %d passports, want exactly her own 1", len(aliceRows))
	}
	if aliceRows[0].ID != aliceIssued.ID {
		t.Fatalf("alice sees passport %s, want her minted %s", aliceRows[0].ID, aliceIssued.ID)
	}
	if aliceRows[0].Label == nil || *aliceRows[0].Label != "alice claude" {
		t.Fatalf("label = %v, want alice claude", aliceRows[0].Label)
	}

	adminRows, err := e.svc.ListPassports(ctx, e.identityFor(e.alice, []string{"admin"}))
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	if len(adminRows) != 2 {
		t.Fatalf("admin sees %d passports, want the workspace's 2", len(adminRows))
	}
}

// A revoked passport stays listed with its revoked_at stamped — the
// Settings surface shows the kill switch took, it does not hide history.
func TestListPassportsShowsRevocation(t *testing.T) {
	e := setupPassports(t)
	ctx := e.ctx()

	id := e.identityFor(e.alice, []string{"rep"})
	issued, err := e.svc.IssuePassport(ctx, id, identity.IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if err := e.svc.RevokePassport(ctx, id, issued.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	rows, err := e.svc.ListPassports(ctx, id)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("listed %d rows, want 1", len(rows))
	}
	if rows[0].RevokedAt == nil {
		t.Fatal("revoked passport lists without revoked_at")
	}
}

// TestRevocationReachesARunAlreadyInFlight — the specification #2780 names.
//
// A runner authenticates a job's passport ONCE, at run start, and caches the
// principal for the life of the run. Per-tool admission re-derived the human's
// RBAC every call, which is what makes "agent ≤ human" a runtime property, and
// never re-asked whether the passport itself was still alive. So revoking a
// credential mid-run stopped nothing until the run ended on its own — and
// revocation is documented as binding "at the next token lookup", which inside
// a run there was not one of.
//
// AdmittedAuthority is that lookup now, and it is the call platform/auth's gate
// makes on every tool. Driven at the seam rather than through the gate because
// this is the question the seam answers; the gate acting on the refusal is
// pinned by its own suite, against a resolver that returns it.
func TestRevocationReachesARunAlreadyInFlight(t *testing.T) {
	e := setupPassports(t)
	ctx := e.ctx()
	id := e.identityFor(e.alice, []string{"rep"})

	issued, err := e.svc.IssuePassport(ctx, id, identity.IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	passport := issued.ID.UUID

	// The run starts. This is what every tool call asks from here on.
	if _, _, err := e.svc.AdmittedAuthority(ctx, e.WS, e.alice, passport); err != nil {
		t.Fatalf("a live passport was refused at admission: %v", err)
	}

	// The operator kills it while the run is in flight.
	if err := e.svc.RevokePassport(ctx, id, issued.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// The next tool call.
	if _, _, err := e.svc.AdmittedAuthority(ctx, e.WS, e.alice, passport); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("admission after revocation answered %v, want not-found — the killed credential is still executing tools", err)
	}
	// And it is the PASSPORT that stopped it, not the human. Alice is
	// untouched: her seat and her grants still resolve, so a check that had
	// merely started refusing everything would fail here instead of passing
	// for the wrong reason.
	if _, err := e.svc.EffectiveRBAC(ctx, e.WS, e.alice); err != nil {
		t.Errorf("the granting human stopped resolving too (%v) — this case would pass whether or not revocation reached the passport", err)
	}
}

// TestAPassportRegrantedElsewhereStopsActingForTheHumanItLeft — the second half
// of the same re-check, and the reason it compares the granting human.
//
// A principal carries the human it was minted for, stamped when the run
// started. "Agent ≤ human" is a runtime property, so a passport that now
// answers to somebody else must not keep admitting calls bounded by the
// authority of the human it left — their seat, their grants, their teams.
func TestAPassportRegrantedElsewhereStopsActingForTheHumanItLeft(t *testing.T) {
	e := setupPassports(t)
	ctx := e.ctx()
	id := e.identityFor(e.alice, []string{"rep"})

	issued, err := e.svc.IssuePassport(ctx, id, identity.IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, _, err := e.svc.AdmittedAuthority(ctx, e.WS, e.bob, issued.ID.UUID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("alice's passport admitted a call on bob's authority: %v", err)
	}
}

// TestAPrincipalWithNoPassportIsAnsweredOnItsHumansAuthority — the exception,
// asserted rather than assumed.
//
// Two production paths mint an agent principal holding no credential: an
// extension job tick and the auto-apply actor, both derived from a live human
// at construction. There is nothing for revocation to reach, so the zero
// passport must resolve on the human alone — and if it did not, both would be
// refused every call.
func TestAPrincipalWithNoPassportIsAnsweredOnItsHumansAuthority(t *testing.T) {
	e := setupPassports(t)
	if _, _, err := e.svc.AdmittedAuthority(e.ctx(), e.WS, e.alice, ids.UUID{}); err != nil {
		t.Errorf("a principal holding no passport was refused (%v) — the extension tick and the auto-apply actor both hold none", err)
	}
}
