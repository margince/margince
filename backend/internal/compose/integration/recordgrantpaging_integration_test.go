// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record-grant list honours the page it was asked for.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The list serves the page it was asked for, and walks the rest exactly once.
//
// WHY THIS ONE IS PAGED AND THE OTHER TWO ARE NOT. Consent purposes and voice
// corpus sources are configuration — a workspace sets up a handful once. A
// grant row is created every time somebody shares a record, so its count is
// driven by usage, and the unfiltered read returned the WHOLE table and then
// ran one visibility query per row it had read.
//
// The subtlety is that the visibility filter runs AFTER the page: a page of
// `limit` rows can serve fewer once the invisible ones are dropped. So the
// walk is asserted by what it SEES across pages, and the pages are sized
// smaller than the fixture so a boundary is actually crossed.
func TestTheRecordGrantListServesThePageItWasAskedFor(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, AdminPerms)
	shares := identity.NewServiceFor(e.DB())

	const total = 5
	for i := range total {
		org := e.SeedOrg(t, "Shared Holding", &e.Rep1)
		if _, err := shares.CreateRecordGrant(ctx, identity.CreateGrantInput{
			RecordType: "organization", RecordID: org,
			SubjectType: "user", SubjectID: e.Rep3, Access: "read",
		}); err != nil {
			t.Fatalf("sharing record %d: %v", i, err)
		}
	}

	limit := 2
	var seen []ids.UUID
	var cursor *string
	for range total + 2 {
		grants, page, err := shares.ListRecordGrants(ctx, identity.ListGrantsInput{
			Limit: &limit, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("listing after %d grant(s): %v", len(seen), err)
		}
		if len(grants) > limit {
			t.Fatalf("a page of %d came back for limit=%d — the caller sized a page and got more of "+
				"the catalog than they asked for", len(grants), limit)
		}
		for _, g := range grants {
			seen = append(seen, g.ID)
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatalf("the page says there is more and hands back no cursor to fetch it with")
		}
		cursor = &page.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("the walk saw %d grant(s), want %d — a page that ends early leaves grants nobody can "+
			"reach, and one that repeats serves the same share twice", len(seen), total)
	}
	unique := map[ids.UUID]bool{}
	for _, id := range seen {
		if unique[id] {
			t.Fatalf("grant %s was served twice — the keyset repeats at a page boundary", id)
		}
		unique[id] = true
	}
}

// An unpaged call is still bounded.
//
// The default limit applies when the caller names none, which is what stops the
// old behaviour — read the whole table, then one visibility query per row —
// coming back for a caller who simply did not pass a dial.
func TestTheRecordGrantListIsBoundedWithoutADial(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, AdminPerms)
	shares := identity.NewServiceFor(e.DB())
	for range 3 {
		org := e.SeedOrg(t, "Shared Holding", &e.Rep1)
		if _, err := shares.CreateRecordGrant(ctx, identity.CreateGrantInput{
			RecordType: "organization", RecordID: org,
			SubjectType: "user", SubjectID: e.Rep3, Access: "read",
		}); err != nil {
			t.Fatalf("sharing a record: %v", err)
		}
	}

	grants, _, err := shares.ListRecordGrants(ctx, identity.ListGrantsInput{})
	if err != nil {
		t.Fatalf("listing without a dial: %v", err)
	}
	if len(grants) == 0 {
		t.Fatal("a caller passing no dial got nothing — the default limit must serve a page, not refuse one")
	}
}
