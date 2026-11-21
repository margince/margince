// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A company pair belongs to the owners of either end, and is ownerless only
// when neither end has an owner.
func TestACompanyPairBelongsToTheOwnersOfEitherEnd(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Alpha Trading GmbH", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep))})
	if err != nil {
		t.Fatalf("create the first company: %v", err)
	}
	second, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Alpha Trading GmbH", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
	if err != nil {
		t.Fatalf("create the same name again: %v", err)
	}

	owned := func(owners *storekit.OwnerSet) (listed, counted int) {
		t.Helper()
		rows, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany, Owners: owners})
		if err != nil {
			t.Fatalf("list the pairs of %+v: %v", owners, err)
		}
		counted, err = e.store.CountOpenDedupePairsOf(ctx, owners, entityCompany)
		if err != nil {
			t.Fatalf("count the pairs of %+v: %v", owners, err)
		}
		return len(rows), counted
	}
	cases := []struct {
		name   string
		owners *storekit.OwnerSet
		want   int
	}{
		{"every owner", nil, 1},
		{"the first end's owner", &storekit.OwnerSet{IDs: []ids.UUID{e.rep}}, 1},
		{"the second end's owner", &storekit.OwnerSet{IDs: []ids.UUID{e.otherRep}}, 1},
		{"an owner of neither end", &storekit.OwnerSet{IDs: []ids.UUID{ids.NewV7()}}, 0},
		{"no owner at all, while an end has one", &storekit.OwnerSet{Unassigned: true}, 0},
	}
	for _, tc := range cases {
		if listed, counted := owned(tc.owners); listed != tc.want || counted != tc.want {
			t.Errorf("%s: listed %d and counted %d pairs, want %d", tc.name, listed, counted, tc.want)
		}
	}

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET owner_id = NULL WHERE id = ANY($1)`, []ids.UUID{ids.UUID(first.Id), ids.UUID(second.Id)})
		return err
	}); err != nil {
		t.Fatalf("clear both owners: %v", err)
	}
	if listed, counted := owned(&storekit.OwnerSet{Unassigned: true}); listed != 1 || counted != 1 {
		t.Errorf("a pair with no owner on either end: listed %d and counted %d under unassigned, want 1", listed, counted)
	}
	if listed, counted := owned(&storekit.OwnerSet{IDs: []ids.UUID{e.rep}}); listed != 0 || counted != 0 {
		t.Errorf("a pair with no owner: listed %d and counted %d for a member, want 0", listed, counted)
	}
}

// The record filter answers the pairs that name one company, and refuses to
// guess the record's type.
func TestThePairsNamingOneCompanyAreFiltered(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Beta Freight GmbH", Source: "manual"})
	if err != nil {
		t.Fatalf("create the first company: %v", err)
	}
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Beta Freight GmbH", Source: "manual"}); err != nil {
		t.Fatalf("create the same name again: %v", err)
	}
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Quartz Observatory", Source: "manual"}); err != nil {
		t.Fatalf("create the unrelated company: %v", err)
	}

	named := ids.UUID(first.Id)
	rows, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany, RecordID: &named})
	if err != nil || len(rows) != 1 {
		t.Fatalf("pairs naming the first company = %d, err %v, want 1", len(rows), err)
	}
	stranger := ids.NewV7()
	rows, _, err = e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany, RecordID: &stranger})
	if err != nil || len(rows) != 0 {
		t.Fatalf("pairs naming a company in no pair = %d, err %v, want 0", len(rows), err)
	}
	var refusal *DedupeInputError
	if _, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{RecordID: &named}); !errors.As(err, &refusal) {
		t.Fatalf("a record filter with no record type answered %v, want a DedupeInputError", err)
	}
}
