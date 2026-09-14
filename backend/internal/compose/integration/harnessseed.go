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

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	// The harness's AdminPerms deliberately carries no `partner` grant, so the
	// seeding acts as a seat that has one. Borrowing the caller's context would
	// make every suite that seeds a partner grow a permission it is not testing.
	seeder := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects: map[string]principal.ObjectGrant{
			"partner": {Create: true, Read: true, Update: true},
			// Becoming a partner also stamps the company's relationship
			// types, so the seat needs the company as well as the programme.
			objCompany: {Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	if _, err := e.Contacts.UpsertPartner(seeder, contacts.UpsertPartnerInput{
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
