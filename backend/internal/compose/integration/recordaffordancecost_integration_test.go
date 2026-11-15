// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRecordAffordancesShareAuthorityOnStoreReads(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Contacts.SetClock(func() time.Time { return now })
	e.Deals.SetClock(func() time.Time { return now })
	pipeline, open, _ := DealFixture(t, e)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Authority contact", &e.Rep1))
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Authority company", &e.Rep1))
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Authority deal", pipeline, open, &e.Rep1))
	cases := []struct {
		name string
		read func(context.Context, bool)
	}{
		{"contact", func(ctx context.Context, writable bool) {
			row, err := e.Contacts.GetContact(ctx, contact, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"company", func(ctx context.Context, writable bool) {
			row, err := e.Contacts.GetCompany(ctx, company, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"deal", func(ctx context.Context, writable bool) {
			row, err := e.Deals.GetDeal(ctx, deal, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"contact page", func(ctx context.Context, writable bool) {
			rows, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("contacts = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"company page", func(ctx context.Context, writable bool) {
			rows, _, err := e.Contacts.ListCompanies(ctx, contacts.ListCompaniesInput{IDs: []ids.UUID{company.UUID}})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("companies = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"deal page", func(ctx context.Context, writable bool) {
			rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("deals = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
	}
	for _, tc := range cases {
		// Both readers keep delete, so company domain checks and child reads match.
		var costs [2]int64
		for i, update := range []bool{false, true} {
			ctx, meter := database.WithStatementMeter(asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: update, deleteGrant: true})))
			tc.read(ctx, update)
			cost := meter.Cost()
			if cost.Statements == 0 || cost.Failed != 0 {
				t.Fatalf("%s: invalid read cost %+v", tc.name, cost)
			}
			costs[i] = cost.Statements
		}
		t.Logf("%s: delete-only=%d, update+delete=%d statements", tc.name, costs[0], costs[1])
		if costs[0] != costs[1] {
			t.Errorf("%s: delete-only read = %d statements, update+delete = %d; adding edit must reuse archive authority", tc.name, costs[0], costs[1])
		}
	}
}

type recordWriteGrants struct {
	update, deleteGrant bool
}

type expectedRecordActions struct {
	writable, archivable bool
}

func recordAffordancePerms(grants recordWriteGrants) principal.Permissions {
	perms := ownRecordArchivePerms()
	perms.RowScope = principal.RowScopeOwn
	for _, object := range []string{objContact, objCompany, objDeal} {
		grant := perms.Objects[object]
		grant.Update, grant.Delete = grants.update, grants.deleteGrant
		perms.Objects[object] = grant
	}
	return perms
}

func requireRecordAffordances(t *testing.T, writable, archivable *bool, want expectedRecordActions) {
	t.Helper()
	if writable == nil || archivable == nil {
		t.Fatal("missing writable or archivable flag")
	}
	if *writable != want.writable || *archivable != want.archivable {
		t.Errorf("writable/archivable = %t/%t, want %t/%t", *writable, *archivable, want.writable, want.archivable)
	}
}
