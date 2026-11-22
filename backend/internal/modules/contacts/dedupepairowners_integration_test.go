// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// The owner and record filters keep the both-sides rule. A pair reaches nobody's
// Mine page or company page unless that reader can open both records.
func TestTheOwnerAndRecordFiltersKeepTheBothSidesRule(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, second := seedCompanyPair(ctx, t, e)
	setRowVisibility(ctx, t, e, `UPDATE company SET owner_id = $1, visibility = 'owner' WHERE id = $2`, &e.rep, first)
	setRowVisibility(ctx, t, e, `UPDATE company SET owner_id = $1, visibility = 'workspace' WHERE id = $2`, &e.otherRep, second)

	asOwner, asColleague := e.asOwnScoped(e.rep), e.asOwnScoped(e.otherRep)
	for _, tc := range []struct {
		name   string
		reader context.Context
		owners *storekit.OwnerSet
		record ids.UUID
		want   int
	}{
		{"the owner of the private end", asOwner, &storekit.OwnerSet{IDs: []ids.UUID{e.rep}}, first, 1},
		{"the colleague who owns the shared end", asColleague, &storekit.OwnerSet{IDs: []ids.UUID{e.otherRep}}, second, 0},
	} {
		listed, _, err := e.store.ListDedupeCandidates(tc.reader, DedupeQueueInput{EntityType: entityCompany, Owners: tc.owners})
		if err != nil || len(listed) != tc.want {
			t.Errorf("%s: Mine lists %d pairs, err %v, want %d", tc.name, len(listed), err, tc.want)
		}
		counted, err := e.store.CountOpenDedupePairsOf(tc.reader, tc.owners, entityCompany)
		if err != nil || counted != tc.want {
			t.Errorf("%s: Mine counts %d pairs, err %v, want %d", tc.name, counted, err, tc.want)
		}
		named, _, err := e.store.ListDedupeCandidates(tc.reader, DedupeQueueInput{EntityType: entityCompany, RecordID: &tc.record})
		if err != nil || len(named) != tc.want {
			t.Errorf("%s: the record read lists %d pairs, err %v, want %d", tc.name, len(named), err, tc.want)
		}
	}

	noGrant := principal.WithActor(e.as(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.otherRep.String(), UserID: e.otherRep,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	})
	if _, _, err := e.store.ListDedupeCandidates(noGrant, DedupeQueueInput{EntityType: entityCompany, RecordID: &first}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the record read without a company grant = %v, want ErrPermissionDenied", err)
	}
}

// A pair is unassigned unless one of its records has a live owner, and a pair
// names only live owners as its deciders. A member who has left owns nothing.
func TestAFormerMembersCompaniesLeaveTheirPairUnassigned(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for range 2 {
		if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Gamma Logistics GmbH", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))}); err != nil {
			t.Fatalf("create a company: %v", err)
		}
	}
	unassigned := &storekit.OwnerSet{Unassigned: true}
	rows, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany, Owners: unassigned})
	if err != nil || len(rows) != 0 {
		t.Fatalf("a pair of a live owner's companies lists %d as unassigned, err %v, want 0", len(rows), err)
	}
	owned, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany})
	if err != nil || len(owned) != 1 || len(owned[0].Owners) != 1 || owned[0].Owners[0] != e.otherRep {
		t.Fatalf("the pair names %+v as its owners, err %v, want the one live owner", owned, err)
	}

	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE app_user SET removed_at = now(), status = 'deactivated' WHERE id = $1`, e.otherRep)
		return err
	}); err != nil {
		t.Fatalf("remove the member: %v", err)
	}
	rows, _, err = e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityCompany, Owners: unassigned})
	if err != nil || len(rows) != 1 || len(rows[0].Owners) != 0 {
		t.Fatalf("a pair of a removed owner's companies lists %d as unassigned with owners %v, err %v, want 1 and none", len(rows), rows, err)
	}
	if counted, err := e.store.CountOpenDedupePairsOf(ctx, unassigned, entityCompany); err != nil || counted != 1 {
		t.Fatalf("the unassigned count = %d, err %v, want 1", counted, err)
	}
}

// The counted first page answers what the list and the separate count did.
// It returns the same pairs, and a total of every pair the reader may see even
// when the page holds fewer.
func TestTheCountedFirstPageMatchesTheListAndTheCount(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, name := range []string{"Alpha Trading GmbH", "Beta Freight GmbH", "Gamma Logistics GmbH"} {
		for range 2 {
			company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep))})
			if err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			setRowVisibility(ctx, t, e, `UPDATE company SET owner_id = $1, visibility = 'owner' WHERE id = $2`, &e.rep, ids.UUID(company.Id))
		}
	}

	for _, tc := range []struct {
		name      string
		reader    context.Context
		limit     int
		wantPage  int
		wantTotal int
	}{
		{"full scope, whole page", ctx, 10, 3, 3},
		{"full scope, truncated page", ctx, 2, 2, 3},
		{"the owner under own scope", e.asOwnScoped(e.rep), 10, 3, 3},
		{"the owner under own scope, truncated", e.asOwnScoped(e.rep), 1, 1, 3},
		{"a colleague who sees neither end", e.asOwnScoped(e.otherRep), 10, 0, 0},
	} {
		in := DedupeQueueInput{EntityTypes: []string{entityCompany}, Limit: tc.limit}
		page, total, err := e.store.ListFirstDedupePage(tc.reader, in)
		if err != nil || len(page) != tc.wantPage || total != tc.wantTotal {
			t.Errorf("%s: page %d, total %d, err %v; want page %d, total %d", tc.name, len(page), total, err, tc.wantPage, tc.wantTotal)
		}
		listed, _, err := e.store.ListDedupeCandidates(tc.reader, in)
		if err != nil || len(listed) != len(page) {
			t.Fatalf("%s: the plain list holds %d, err %v, want %d", tc.name, len(listed), err, len(page))
		}
		for i := range listed {
			if listed[i].ID != page[i].ID {
				t.Errorf("%s: row %d is %s, the plain list has %s", tc.name, i, page[i].ID, listed[i].ID)
			}
		}
		counted, err := e.store.CountOpenDedupePairsOf(tc.reader, nil, entityCompany)
		if err != nil || counted != total {
			t.Errorf("%s: the separate count is %d, err %v, want the page's total %d", tc.name, counted, err, total)
		}
	}
}
