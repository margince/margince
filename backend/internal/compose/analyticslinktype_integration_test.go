// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal falls in one group by the link types it holds with other live deals.
// The groups add up to every deal. A deal linked only to a company holds none.
func TestGroupingDealsByLinkTypeCountsEveryDealOnce(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals())
	var seeded []ids.UUID
	for _, name := range []string{"Original", "Follow-up", "Renewal", "Alone", "Linked to a company"} {
		deal, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{Name: name, PipelineID: pipeline, StageID: open})
		if err != nil {
			t.Fatal(err)
		}
		seeded = append(seeded, ids.UUID(deal.Id))
	}
	link := func(fromType string, from ids.UUID, toType string, to ids.UUID, linkType string) {
		t.Helper()
		e.WsExec(t, `INSERT INTO record_link (from_type, from_id, to_type, to_id, link_type, captured_by)
			VALUES ($1, $2, $3, $4, $5, 'test')`, fromType, from, toType, to, linkType)
	}
	link("deal", seeded[1], "deal", seeded[0], "follow_up_of")
	link("deal", seeded[1], "deal", seeded[2], "renews")
	link("deal", seeded[4], "company", e.SeedCompany(t, "Acme", nil), "related_to")

	answer, err := dealsByColumn(e.Admin(), e, "link_type")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	total := 0
	for _, row := range answer.Rows {
		got[fmt.Sprint(row["link_type"])] = countOf(t, row)
		total += countOf(t, row)
	}
	want := map[string]int{"follow_up_of": 1, "follow_up_of, renews": 1, "renews": 1, "none": 2}
	for key, n := range want {
		if got[key] != n {
			t.Errorf("group %q counts %d, want %d (all groups: %v)", key, got[key], n, got)
		}
	}
	if total < len(seeded) {
		t.Errorf("the groups count %d deals, want at least the %d seeded", total, len(seeded))
	}
}
