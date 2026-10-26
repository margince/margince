// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

func (e *dedupeEnv) markColumns(ctx context.Context, t *testing.T, id ids.ContactID) (mark, setBy *string) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, e.store.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT relationship_mark, relationship_mark_set_by FROM contact WHERE id = $1`, id).Scan(&mark, &setBy)
	}); err != nil {
		t.Fatalf("reading the mark columns: %v", err)
	}
	return mark, setBy
}

func TestAHumanSetsAndClearsTheMarkAndTheSetterFollowsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Dana Keller", Source: "manual",
		Emails: []ContactEmailInput{{Email: "dana@pitch.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))

	mark := relationshipmark.VendorMarketing
	marked, err := e.store.UpdateContact(ctx, id, UpdateContactInput{RelationshipMark: &mark})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if marked.RelationshipMark == nil || *marked.RelationshipMark != crmcontracts.ContactRelationshipMarkVendorMarketing {
		t.Fatalf("the update returned mark %v, want vendor_marketing", marked.RelationshipMark)
	}
	got, by := e.markColumns(ctx, t, id)
	if got == nil || *got != "vendor_marketing" || by == nil || *by != "human:"+e.rep.String() {
		t.Fatalf("stored mark = %v set by %v, want vendor_marketing set by human:%s", got, by, e.rep)
	}

	cleared, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Clear: []string{"relationship_mark"}})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.RelationshipMark != nil {
		t.Fatalf("the update returned mark %v after a clear", *cleared.RelationshipMark)
	}
	got, by = e.markColumns(ctx, t, id)
	if got != nil || by != nil {
		t.Fatalf("after a clear the columns are %v / %v, want both NULL (the table requires both or neither)", got, by)
	}
}

func TestAnUnknownMarkIsRefusedBeforeTheDatabaseSeesIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Pat Doe", Source: "manual"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	bogus := relationshipmark.Mark("friend")
	_, err = e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(created.Id)), UpdateContactInput{RelationshipMark: &bogus})
	if err == nil {
		t.Fatal("an unknown mark was accepted")
	}
}
