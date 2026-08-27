// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// A subscriber who may READ a contract receives its events.
//
// A contract's visibility is inherited rather than owned (ADR-0109 §8):
// GET /contracts/{id} asks for contract.read and then borrows the anchor's
// ROW-SCOPE predicate, without demanding the anchor's object grant. Delivery
// asked for that grant as well, so a custom role holding contract.read and not
// deal.read could open a contract over HTTP and silently receive none of its
// four subscribed events. Fails closed, so a delivery gap rather than a leak —
// and still two paths answering one question differently.
//
// The anchor must also be LIVE, which is the same divergence in the direction
// that DOES leak: an archived deal keeps its foreign key and its grants, so a
// probe ignoring archival would deliver events about a contract whose own read
// answers 404.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type contractVisEnv struct {
	store *Store
	owner *pgx.Conn
	pool  *pgxpool.Pool
	ws    ids.UUID
	user  ids.UUID
	org   ids.UUID
	deal  ids.UUID
}

func setupContractVis(t *testing.T) *contractVisEnv {
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
	// Before any seed: EnsureSchema rebuilds whenever it cannot prove the
	// database is a fresh lane clone, and a row written first would be dropped.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &contractVisEnv{owner: owner, ws: ids.NewV7(), user: ids.NewV7(), org: ids.NewV7(), deal: ids.NewV7()}
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace (id) VALUES ($1)`, []any{e.ws}},
		{
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
			[]any{e.user, "rep-" + e.user.String() + "@vis.test"},
		},
		{`INSERT INTO organization (id, display_name, source, captured_by)
		  VALUES ($1, 'Acme', 'manual', 'human:test')`, []any{e.org}},
	} {
		if _, err := owner.Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.pool = pool
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)), nil)
	return e
}

// seedDealAndContract plants an anchor the caller owns and a contract on it.
func (e *contractVisEnv) seedDealAndContract(t *testing.T) ids.UUID {
	t.Helper()
	ctx := context.Background()
	contract, pipeline, stage := ids.NewV7(), ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO pipeline (id, name, position) VALUES ($1, $2, 1)`, pipeline, "P "+pipeline.String()); err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, position) VALUES ($1, $2, 'Open', 1)`,
		stage, pipeline); err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO deal (id, organization_id, name, pipeline_id, stage_id, owner_id, source, captured_by)
		VALUES ($1, $2, 'A deal', $3, $4, $5, 'manual', 'human:test')`,
		e.deal, e.org, pipeline, stage, e.user); err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO contract (id, organization_id, deal_id, title, value_basis, status, source, captured_by)
		VALUES ($1, $2, $3, 'An agreement', 'total', 'active', 'manual', 'human:test')`,
		contract, e.org, e.deal); err != nil {
		t.Fatalf("seeding the contract: %v", err)
	}
	return contract
}

// asHolding is a rep whose row scope is their own records, holding exactly the
// object grants named. The point of the fixture is what it does NOT hold.
func (e *contractVisEnv) asHolding(objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  objects,
			RowScope: principal.RowScopeOwn,
		},
	})
}

func TestAContractSubjectNeedsNoGrantOnItsAnchor(t *testing.T) {
	e := setupContractVis(t)
	contract := e.seedDealAndContract(t)

	// contract.read and NOTHING on the deal — the custom role the seeded ones
	// all avoid by holding both.
	visible, err := e.store.contractVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"contract": {Read: true}}), contract,
	)
	if err != nil {
		t.Fatalf("probing the contract's visibility: %v", err)
	}
	if !visible {
		t.Error("a subscriber who may READ this contract over HTTP receives none of its events — " +
			"delivery asked for a grant on the anchor that the read does not")
	}
}

// The grant it does need.
func TestAContractSubjectStillNeedsContractRead(t *testing.T) {
	e := setupContractVis(t)
	contract := e.seedDealAndContract(t)

	visible, err := e.store.contractVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"deal": {Read: true}}), contract,
	)
	if err != nil {
		t.Fatalf("probing the contract's visibility: %v", err)
	}
	if visible {
		t.Error("a subscriber with no contract.read received a contract's events")
	}
}

// An archived anchor is the divergence in the direction that leaks: the HTTP
// read answers 404 through it, so delivery must not carry on.
func TestAContractOnAnArchivedAnchorIsNotDelivered(t *testing.T) {
	e := setupContractVis(t)
	contract := e.seedDealAndContract(t)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE id = $1`, e.deal); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	visible, err := e.store.contractVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"contract": {Read: true}}), contract,
	)
	if err != nil {
		t.Fatalf("probing the contract's visibility: %v", err)
	}
	if visible {
		t.Error("a contract whose anchor is archived was delivered — its own read answers 404, " +
			"so this hands a subscriber what the API refuses them")
	}
}
