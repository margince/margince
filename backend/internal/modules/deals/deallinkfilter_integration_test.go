// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The deal list filtered by a link type, over rows the typed-link writer made.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADealListNarrowsToDealsHoldingALinkOfOneType(t *testing.T) {
	e := setupConfigEnv(t)
	stage := openStage(t, e, SemanticOpen)
	first, second, third := seedDealInStage(t, e, stage), seedDealInStage(t, e, stage), seedDealInStage(t, e, stage)
	company := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(),
		`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Acme', 'seed', 'system')`, company); err != nil {
		t.Fatal(err)
	}
	link := func(fromType string, from ids.UUID, toType string, to ids.UUID, linkType string) {
		t.Helper()
		if _, err := e.owner.Exec(t.Context(), `
			INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by)
			VALUES ($1, $2, $3, $4, $5, 'test')`, fromType, from, toType, to, linkType); err != nil {
			t.Fatal(err)
		}
	}
	link("deal", first.UUID, "deal", second.UUID, "follow_up_of")
	link("deal", third.UUID, "company", company, "follow_up_of")

	filtered := func(linkType string) []ids.UUID {
		t.Helper()
		deals, _, err := e.store.ListDeals(e.asDealWriter(), ListDealsInput{LinkType: &linkType})
		if err != nil {
			t.Fatalf("ListDeals(%s): %v", linkType, err)
		}
		var got []ids.UUID
		for _, d := range deals {
			got = append(got, ids.UUID(d.Id))
		}
		slices.SortFunc(got, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
		return got
	}
	want := []ids.UUID{first.UUID, second.UUID}
	slices.SortFunc(want, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
	if got := filtered("follow_up_of"); !slices.Equal(got, want) {
		t.Errorf("deals with a follow_up_of link = %v, want both ends of the deal-to-deal link and not the deal linked to a company (%v)", got, want)
	}
	if got := filtered("renews"); len(got) != 0 {
		t.Errorf("deals with a renews link = %v, want none", got)
	}

	if _, err := e.owner.Exec(t.Context(), `UPDATE deal SET archived_at = now() WHERE id = $1`, second.UUID); err != nil {
		t.Fatal(err)
	}
	if got := filtered("follow_up_of"); len(got) != 0 {
		t.Errorf("after archiving one end the filter still returns %v, want none", got)
	}
}
