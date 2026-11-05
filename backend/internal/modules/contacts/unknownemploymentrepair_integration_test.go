// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func unknownRepairFixture(t *testing.T) (*dedupeEnv, context.Context, relationshipRow) {
	t.Helper()
	e := setupDedupe(t)
	ctx := e.as()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx = principal.WithActor(ctx, actor)
	subject, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Colleague", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	employer, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Employer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(subject.Id))
	company := ids.From[ids.CompanyKind](ids.UUID(employer.Id))
	row, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{Kind: employmentKind, ContactID: &contact, CompanyID: &company, EmploymentStatus: new("unknown"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var stored relationshipRow
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		stored, err = e.store.visibleRelationship(ctx, tx, ids.UUID(row.ID))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return e, ctx, stored
}

func TestUnknownEmploymentRepairKeepsAnEditAfterSelection(t *testing.T) {
	e, ctx, row := unknownRepairFixture(t)
	if _, err := e.store.UpdateRelationship(ctx, row.ID, UpdateRelationshipInput{Role: new("Advisor"), IfVersion: &row.Version}); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(ctx, "system:employment_repair_test")
	err := e.store.tx(system, func(tx pgx.Tx) error {
		changed, err := e.store.repairUnknownEmploymentLinkTx(system, tx, row.ID, ids.NewV7().String())
		if !changed {
			t.Error("repair skipped a still-eligible edited link")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.store.GetRelationship(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EmploymentStatus == nil || *got.EmploymentStatus != "current" || got.Role == nil || *got.Role != "Advisor" {
		t.Fatalf("edit changed: %+v", got)
	}
}

func TestUnknownEmploymentRepairAdvancesPastAConflictingLiveSlot(t *testing.T) {
	e, ctx, row := unknownRepairFixture(t)
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{Kind: employmentKind, ContactID: row.ContactID, CompanyID: row.CompanyID, EmploymentStatus: new("current"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(ctx, "system:employment_repair_test")
	err := e.store.tx(system, func(tx pgx.Tx) error {
		page, err := e.store.RepairUnknownEmploymentTx(system, tx, ids.Nil, 1, ids.NewV7().String())
		if err != nil {
			return err
		}
		if len(page.Scanned) != 1 || page.Scanned[0] != row.ID || page.Changed != 0 {
			t.Fatalf("conflict page: %+v", page)
		}
		next, err := e.store.RepairUnknownEmploymentTx(system, tx, page.Scanned[0], 1, ids.NewV7().String())
		if len(next.Scanned) != 0 {
			t.Errorf("cursor reread conflict: %+v", next)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.store.GetRelationship(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EmploymentStatus == nil || *got.EmploymentStatus != "unknown" {
		t.Fatal("conflicting link changed")
	}
}
