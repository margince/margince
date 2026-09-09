// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An account's contract list is ordered by TERM, and pages across the boundary
// between dated and undated agreements without repeating or skipping one.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedTermFixture writes four agreements on one account, created in an order
// deliberately opposite to their terms so creation order cannot pass for term
// order: the newest term is written FIRST and the oldest LAST.
//
// Two carry no start date, which is the ordinary shape of an imported
// agreement and the case the null placement is about.
func seedTermFixture(t *testing.T, e *Env) (org ids.OrganizationID, byTerm []string) {
	t.Helper()
	orgID := orgIDOf(e.SeedOrg(t, "Terms GmbH", nil))
	day := func(y int, m time.Month, d int) *time.Time {
		when := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &when
	}
	for _, c := range []struct {
		title    string
		startsOn *time.Time
	}{
		{"2026 term", day(2026, time.January, 1)},
		{"2024 term", day(2024, time.January, 1)},
		{"undated, written third", nil},
		{"undated, written fourth", nil},
	} {
		seedContract(t, e, contracts.CreateContractInput{
			OrganizationID: orgID, Title: c.title, StartsOn: c.startsOn, ValueBasis: "total",
		})
	}
	// Dated terms newest first, then the undated tail newest-written first —
	// created_at DESC is the only ordering left among agreements that name no
	// term, and it is the tiebreak the ORDER BY falls through to.
	return orgID, []string{"2026 term", "2024 term", "undated, written fourth", "undated, written third"}
}

// The list answers "which agreement is current" — by term, not by write time.
//
// The fixture writes the newest term FIRST, so a list ordered by creation puts
// it last and this fails. That inversion is the whole point: with the two
// orderings agreeing, a test cannot tell which one produced the page.
func TestTheContractListIsOrderedByTermNotByWriteTime(t *testing.T) {
	e := Setup(t)
	org, wantOrder := seedTermFixture(t, e)

	page, err := e.Contracts.ListOrganizationContracts(e.Admin(), contracts.ListContractsInput{
		OrganizationID: org,
	})
	if err != nil {
		t.Fatalf("listing the account's contracts: %v", err)
	}

	got := make([]string, 0, len(page.Data))
	for _, c := range page.Data {
		got = append(got, c.Title)
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("the list answered %d agreement(s), want %d — %v", len(got), len(wantOrder), got)
	}
	for i := range wantOrder {
		if got[i] != wantOrder[i] {
			t.Fatalf("the list reads %v, want %v — an agreement is presented as current on the strength "+
				"of when its row was written rather than when its term begins", got, wantOrder)
		}
	}
}

// Paging across the dated/undated boundary serves every agreement exactly once.
//
// THIS IS THE CASE THE NULL HANDLING IS FOR. Under `starts_on DESC NULLS LAST`
// every dated term precedes every undated one, so the continuation predicate
// asks a different question on each side of that boundary. A single tuple
// comparison cannot express it — `(starts_on, …) < (…)` is NULL-valued the
// moment either side is NULL, so the list would end early with rows remaining,
// and the reader would never learn that agreements exist below the fold.
//
// A limit of 1 walks the boundary a row at a time, so the page that crosses it
// is exercised rather than hoped for.
func TestPagingContractsCrossesTheUndatedBoundaryExactlyOnce(t *testing.T) {
	e := Setup(t)
	org, wantOrder := seedTermFixture(t, e)

	var seen []string
	var cursor *string
	for range len(wantOrder) + 2 {
		limit := 1
		page, err := e.Contracts.ListOrganizationContracts(e.Admin(), contracts.ListContractsInput{
			OrganizationID: org, Limit: &limit, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("listing page after %v: %v", seen, err)
		}
		for _, c := range page.Data {
			seen = append(seen, c.Title)
		}
		if !page.Page.HasMore {
			break
		}
		if page.Page.NextCursor == nil {
			t.Fatalf("the page after %v says there is more and hands back no cursor to fetch it with", seen)
		}
		cursor = page.Page.NextCursor
	}

	if len(seen) != len(wantOrder) {
		t.Fatalf("paging one at a time saw %d agreement(s), want %d — %v. A page that ends early leaves "+
			"agreements the reader can never reach, and one that repeats serves the same term twice",
			len(seen), len(wantOrder), seen)
	}
	for i := range wantOrder {
		if seen[i] != wantOrder[i] {
			t.Fatalf("paging saw %v, want %v — the continuation predicate does not place undated terms "+
				"where the ORDER BY puts them", seen, wantOrder)
		}
	}
}
