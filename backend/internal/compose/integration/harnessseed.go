// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The row fixtures every suite seeds from, split out of harness.go when that
// file crossed the file-length cap (margince/margince#2163).
//
// One concept: put a row in the database and answer its id. Env construction,
// the principal builders and the workspace-bound exec helpers stay next door —
// a suite reaches for exactly one of those three things at a time, and the
// split is along that grain rather than at whatever line the cap fell on.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The id wideners assert a harness-seeded untyped id as the entity a contacts-store
// call targets — the suites' spelling of the contracts-edge ids.From widening. The
// harness keeps its fixture ids untyped so every module's suite can share them,
// and each suite widens at the call it makes.
//
// Only ContactIDOf is exported, because integration/channels widens contact ids from
// outside this package. The other three have no caller beyond it, and a suite
// package that later needs one exports it then.

// ContactIDOf widens a harness fixture id to a contact id.
func ContactIDOf(u ids.UUID) ids.ContactID { return ids.From[ids.ContactKind](u) }

// companyIDOf widens a harness fixture id to a company id.
func companyIDOf(u ids.UUID) ids.CompanyID { return ids.From[ids.CompanyKind](u) }

// leadIDOf widens a harness fixture id to a lead id.
func leadIDOf(u ids.UUID) ids.LeadID       { return ids.From[ids.LeadKind](u) }
func projectIDOf(u ids.UUID) ids.ProjectID { return ids.From[ids.ProjectKind](u) }

// userIDPtr types an optional harness user id (Env keeps its fixture ids
// untyped so every module's suite can use them) for contacts's typed inputs.
func userIDPtr(owner *ids.UUID) *ids.UserID {
	if owner == nil {
		return nil
	}
	id := ids.From[ids.UserKind](*owner)
	return &id
}

// SeedContact creates a contact owned by the given user (nil = ownerless),
// acting as admin.
func (e *Env) SeedContact(t *testing.T, name string, owner *ids.UUID) ids.UUID {
	t.Helper()
	p, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, OwnerID: userIDPtr(owner), Source: "manual"})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(p.Id)
}

// SeedCompany creates a company owned by the given user, acting as admin.
func (e *Env) SeedCompany(t *testing.T, name string, owner *ids.UUID) ids.UUID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: name, OwnerID: userIDPtr(owner),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(company.Id)
}

// SeedPartnerCompany creates a company and gives it a partner programme, so
// a deal may name it.
//
// A deal's partner must BE a partner (the store refuses any other company,
// because commission prices from the margin tier on that row). A fixture that
// pointed a deal at a plain company was writing a row the product itself
// will not write, which is the shape of test that proves nothing.
//
// The tier is optional: a partner with none is a real and common state — the
// arrangement exists, the rate has not been agreed — and accrual treats it as
// earning nothing rather than as an error.
func (e *Env) SeedPartnerCompany(t *testing.T, name string, tier *string, owner *ids.UUID) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, name, owner)
	if _, err := e.Contacts.UpsertPartner(e.PartnerSeat(), contacts.UpsertPartnerInput{
		CompanyID:   ids.From[ids.CompanyKind](company),
		PartnerRole: "consulting",
		MarginTier:  tier,
	}); err != nil {
		t.Fatalf("giving %s a partner programme: %v", name, err)
	}
	return company
}

// SeedCompanyAs creates an ownerless company in a SECOND workspace, under
// that workspace's own context — unlike SeedCompany, which always writes the
// harness's primary workspace as e.Admin().
//
// It names the workspace as well as the ctx because the row lands wherever the
// STORE is bound: the harness's own store would stamp the second tenant's ids
// into the first tenant's transaction, which RLS refuses.
func (e *Env) SeedCompanyAs(ctx context.Context, t *testing.T, ws ids.UUID, name string) ids.UUID {
	t.Helper()
	company, err := e.ContactsFor(ws).CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: name})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(company.Id)
}

// SeedDeal creates a deal owned by the given user, acting as admin.
func (e *Env) SeedDeal(t *testing.T, name string, pipeline ids.PipelineID, stage ids.StageID, owner *ids.UUID) ids.UUID {
	t.Helper()
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, OwnerID: userIDPtr(owner),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(d.Id)
}

// MakeCapturePrivate turns a seeded contact or company into a
// capture-private row — `visibility='owner'`, owned by the given user — the
// state a connector leaves an unpromoted contact in. Contact, company,
// lead and deal are otherwise readable by every seat of the workspace, so
// this is the ONE way a test still has to put an identity row out of a
// caller's read scope; a test about row scope on a commercial table seeds a
// project instead.
func (e *Env) MakeCapturePrivate(t *testing.T, table string, id, owner ids.UUID) {
	t.Helper()
	if table != objContact && table != objCompany {
		t.Fatalf("MakeCapturePrivate: %s carries no visibility column", table)
	}
	e.WsExec(t, `UPDATE `+table+` SET visibility = 'owner', owner_id = $2 WHERE id = $1`, id, owner)
}

// SeedScrubTombstone stamps an erase tombstone on one record's own audit spine,
// dated `at`.
//
// Raw rather than through the eraser, and deliberately: the eraser also ARCHIVES
// its subject and cascades over its links, and an archived record is withheld or
// refused on those grounds whatever any boundary says. A tombstone on a LIVE
// record is therefore the only seed that isolates the erasure boundary itself,
// and no product path writes one for a company at all — so this is the
// fixture that makes a boundary filter falsifiable rather than merely present.
func (e *Env) SeedScrubTombstone(t *testing.T, entityType string, id ids.UUID, at time.Time) {
	t.Helper()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		VALUES ($1, 'system', 'system', 'erase', $2, $3, $4)`,
		ids.NewV7(), entityType, id, at)
}

// seedSystemRoleRows lays down the SHIPPED role set for a fresh Env.
//
// Without it every seat here resolves through identity.EffectiveAuthority to no
// permissions at all, so any job that binds a per-rep principal stops at its
// first authorization check — and a test written against that passes while
// driving none of the behaviour it names.
//
// Rows only: NOTHING is assigned, not even the admin role Bootstrap gives its
// first user. A seat's authority is the thing most cases here are about, and
// several use an UNGRANTED seat on purpose — that one such seat must not cost a
// workspace its morning brief is a real guarantee held by a real test, and
// seeding an assignment retires it silently. A case that wants a role asks for
// it by name through GrantRole.
func seedSystemRoleRows(ctx context.Context, t *testing.T, owner *pgx.Conn) {
	t.Helper()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.SeedSystemRoles(ctx, tx); err != nil {
		//craft:ignore swallowed-errors the seed error is the one to report; a rollback that also fails adds nothing
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// GrantRole gives a seat one of the SHIPPED roles, by key, the way an admin
// changing somebody's role does.
//
// The point is what it unlocks: a job that binds a per-rep principal and
// resolves its authority through identity.EffectiveAuthority gets real
// permissions instead of none, so the test drives the behaviour rather than
// stopping at the first gate. Until this existed, such a test passed
// identically with the code under test rearranged — the permission check
// short-circuited before either arrangement reached anything worth asserting.
//
// SHIPPED means `is_system`, and both statements say so. A fixture that seeds
// its own role is free to name it anything, including a shipped key — and this
// helper promising a shipped document while handing over that fixture's own is
// the exact substitution it exists to prevent. Without the filter the promise
// is in the comment and nowhere else.
//
// An unknown key is FATAL rather than a no-op — and a key that names only a
// custom role is unknown here, by the same rule. A typo, or a borrowed name,
// that granted nothing would leave the test in exactly the state this helper
// exists to escape, and passing, which is the one failure a fixture must not
// have.
func (e *Env) GrantRole(t *testing.T, user ids.UUID, roleKey string) {
	t.Helper()
	if !e.tryGrantRole(t, user, roleKey) {
		t.Fatalf("no shipped role is called %q, so %s was granted nothing and every authority "+
			"check below would refuse them — which is what this call exists to prevent", roleKey, user)
	}
}

// tryGrantRole is GrantRole without the fatal: it answers whether the seat now
// holds the role, so a case can assert the REFUSAL rather than be ended by it.
//
// Split out because the refusal is the half worth testing and t.Fatalf cannot
// be asserted against. Without it the is_system predicate below was covered by
// nothing: removing it left every case green, because the only test reaching
// for a custom key went through holdsRole, whose own filter answered instead.
// A review pass caught that; planting it confirmed it.
func (e *Env) tryGrantRole(t *testing.T, user ids.UUID, roleKey string) bool {
	t.Helper()
	var assigned int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			WITH granted AS (
			  INSERT INTO role_assignment (role_id, user_id)
			  SELECT id, $2 FROM role WHERE key = $1 AND is_system
			  ON CONFLICT DO NOTHING
			  RETURNING 1
			)
			SELECT count(*) FROM granted`, roleKey, user).Scan(&assigned)
	}); err != nil {
		t.Fatalf("granting %s the %q role: %v", user, roleKey, err)
	}
	// An insert that touched no row is either "already held" or "no such
	// shipped role", and only the second is a refusal.
	return assigned > 0 || e.holdsRole(t, user, roleKey)
}

// holdsRole answers whether the seat already carries the role, which is the
// only honest reading of an insert that touched no row.
func (e *Env) holdsRole(t *testing.T, user ids.UUID, roleKey string) bool {
	t.Helper()
	var held bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT EXISTS (
			  SELECT 1 FROM role_assignment ra JOIN role r ON r.id = ra.role_id
			   WHERE ra.user_id = $1 AND r.key = $2 AND r.is_system)`, user, roleKey).Scan(&held)
	}); err != nil {
		t.Fatalf("reading %s's roles: %v", user, err)
	}
	return held
}
