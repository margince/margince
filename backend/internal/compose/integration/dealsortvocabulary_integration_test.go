// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A column the deals list SHOWS is a column the deals list SORTS BY.
//
// The list draws seven columns and its sort vocabulary reached three of them,
// so four headers were dead controls a reader had to learn to ignore — and the
// frontend said so in a comment beside each one rather than being able to fix
// it, because the vocabulary was pinned to a spec set written before the
// columns were. Lars ruled on 2026-08-21 that the rule is the columns, for
// every list in the product.
//
// Three of them are not columns of `deal` at all. A stage orders by its
// position in its PIPELINE — alphabetical stages are the funnel shuffled — and
// the two companies order by the referenced company's name. Each is held
// here, and so is the rule that makes a reference sort safe to offer: ordering
// by a value is reading it, so a company this reader may not open must not
// order the page by the name it is being refused.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestTheDealsListSortsByTheNameColumnItDraws: alphabetical, which a list of
// deals had no way to offer at all.
func TestTheDealsListSortsByTheNameColumnItDraws(t *testing.T) {
	f := setupDealCFV(t)
	// Seeded out of order, so a pass cannot come from insertion order — which
	// is what the default `-created_at` sort would give.
	carla := f.seedScoredDeal(t, "Carla renewal", nil)
	alma := f.seedScoredDeal(t, "Alma expansion", nil)
	brig := f.seedScoredDeal(t, "Brig retainer", nil)

	ascending := "name"
	got, _ := f.listDealIDs(t, deals.ListDealsInput{Sort: &ascending})
	assertIDOrder(t, got, []ids.UUID{alma, brig, carla}, "name ascending")

	descending := "-name"
	got, _ = f.listDealIDs(t, deals.ListDealsInput{Sort: &descending})
	assertIDOrder(t, got, []ids.UUID{carla, brig, alma}, "name descending")
}

// TestTheDealsListSortsByTheStatusColumnItDraws: by the stored value, so the
// three groups sit together.
//
// Alphabetical rather than by lifecycle — lost, open, won — because status is a
// text column and the machinery orders by the column. Asserted as the order it
// actually produces rather than the order a reader might expect, because a test
// that wanted the lifecycle order would be asking for a second copy of the
// vocabulary on the sorting side.
func TestTheDealsListSortsByTheStatusColumnItDraws(t *testing.T) {
	e := Setup(t)
	pipeline, openStage, wonStage := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	openDeal := e.SeedDeal(t, "Still open", pipeline, openStage, &e.Rep1)
	wonDeal := e.SeedDeal(t, "Closed won", pipeline, openStage, &e.Rep1)
	// Through the real transition: a status written by hand would order
	// correctly over a column no writer ever puts that value in.
	if _, err := e.Deals.AdvanceDeal(ctx, ids.From[ids.DealKind](wonDeal), wonInput(wonStage)); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	listed := func(spec string) []ids.UUID {
		t.Helper()
		rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec})
		if err != nil {
			t.Fatalf("ListDeals(sort=%s): %v", spec, err)
		}
		out := make([]ids.UUID, len(rows))
		for i, d := range rows {
			out[i] = ids.UUID(d.Id)
		}
		return out
	}

	// "open" < "won"
	assertIDOrder(t, listed("status"), []ids.UUID{openDeal, wonDeal}, "status ascending")
	assertIDOrder(t, listed("-status"), []ids.UUID{wonDeal, openDeal}, "status descending")
}

// The refusal still refuses. Widening a vocabulary is the shape most likely to
// widen it too far — a sort that reached any column at all would let a caller
// order by a column the list does not publish, and the refusal is what keeps
// the vocabulary a vocabulary.
func TestTheDealsListStillRefusesAFieldItDoesNotPublish(t *testing.T) {
	f := setupDealCFV(t)
	f.seedScoredDeal(t, "Only deal", nil)

	// A real column of `deal`, deliberately: refusing a nonsense name proves
	// nothing about a vocabulary that might simply be "any column".
	notPublished := "captured_by"
	if _, _, err := f.store.ListDeals(f.ctx, deals.ListDealsInput{Sort: &notPublished}); err == nil {
		t.Fatal("the list sorted by a column it does not publish — the vocabulary has stopped being one, and a caller can now order by anything the table holds")
	}
}

// dealsIn lists the deals this caller sees under one sort spec, in order.
func dealsIn(ctx context.Context, t *testing.T, e *Env, spec string) []ids.UUID {
	t.Helper()
	rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec})
	if err != nil {
		t.Fatalf("ListDeals(sort=%s): %v", spec, err)
	}
	out := make([]ids.UUID, len(rows))
	for i, d := range rows {
		out[i] = ids.UUID(d.Id)
	}
	return out
}

// A stage sorts by its place in the pipeline, which is the only order anybody
// sorting by stage means.
//
// The seeded pipeline's stages are deliberately NOT in alphabetical order, so a
// sort that fell back to the name would put them the other way round — which is
// how the assertion tells the two apart.
func TestTheDealsListSortsStagesByPipelineOrderAndNotByName(t *testing.T) {
	e := Setup(t)
	pipeline, _, _ := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	stages, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatalf("reading the seeded pipeline: %v", err)
	}
	// Open stages only: a deal cannot be created on Won or Lost, which are
	// reached by advancing rather than by filing.
	var open []crmcontracts.Stage
	for _, st := range *stages.Stages {
		if st.Semantic == "open" {
			open = append(open, st)
		}
	}
	if len(open) < 3 {
		t.Fatalf("the seeded pipeline has %d open stages; this case needs three", len(open))
	}
	// The case turns on the two orders disagreeing. Asserted rather than
	// assumed: rename the seeded stages into alphabetical order and this test
	// would pass over a sort that had fallen back to the name.
	byName := make([]string, 0, len(open))
	for _, st := range open {
		byName = append(byName, st.Name)
	}
	if slices.IsSorted(byName) {
		t.Fatalf("the seeded open stages %v are already in alphabetical order, so this case "+
			"cannot tell pipeline order from name order", byName)
	}

	// One deal per stage, seeded LAST-stage-first so insertion order is the
	// reverse of the answer and the default sort cannot produce it either.
	want := make([]ids.UUID, len(open))
	for i := len(open) - 1; i >= 0; i-- {
		st := ids.From[ids.StageKind](ids.UUID(open[i].Id))
		want[i] = e.SeedDeal(t, "Deal in "+open[i].Name, pipeline, st, &e.Rep1)
	}

	assertIDOrder(t, dealsIn(ctx, t, e, "stage_id"), want, "stage ascending (pipeline order)")

	reversed := make([]ids.UUID, 0, len(want))
	for i := len(want) - 1; i >= 0; i-- {
		reversed = append(reversed, want[i])
	}
	assertIDOrder(t, dealsIn(ctx, t, e, "-stage_id"), reversed, "stage descending")
}

// A reference sorts by the referenced company's NAME, which is what a reader
// clicking the column is asking for — the id it is named after orders nothing
// anybody can see.
func TestTheDealsListSortsItsCompanyColumnsByTheCompanyName(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	// Named so that ordering by the company disagrees with ordering by the
	// deal's own name, which is the only way to tell which one answered.
	zeta := e.SeedCompany(t, "Zeta Holding", &e.Rep1)
	alma := e.SeedCompany(t, "Alma Werke", &e.Rep1)
	first := seedDealForCompany(t, e, "A deal", pipeline, open, zeta)
	second := seedDealForCompany(t, e, "B deal", pipeline, open, alma)

	assertIDOrder(t, dealsIn(ctx, t, e, "company_id"),
		[]ids.UUID{second, first}, "company ascending — Alma before Zeta")
	assertIDOrder(t, dealsIn(ctx, t, e, "-company_id"),
		[]ids.UUID{first, second}, "company descending")
}

// A company this reader may not open orders the page by NOTHING.
//
// Ordering by a value is reading it. A page ordered by a name the reader is
// refused would disclose it through the order — read the list ascending and
// descending and the hidden company's position tells you where its name falls
// in the alphabet. It sorts into the NULL tail instead, which is where every
// deal whose company the reader cannot see lands together, and which says the
// same thing the row itself says: nothing.
func TestAnUnreadableCompanyOrdersTheDealsListByNothing(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	// "Alma" sorts first of the three by name. Hidden from this reader, it must
	// sort last instead — with the deal that names no company at all.
	hidden := e.SeedCompany(t, "Alma Werke", &e.Rep3)
	mid := e.SeedCompany(t, "Mercator", &e.Rep1)
	last := e.SeedCompany(t, "Zeta Holding", &e.Rep1)

	secret := seedDealForCompany(t, e, "Deal with the hidden company", pipeline, open, hidden)
	// Made private AFTER the deal is filed, which is the real sequence: a
	// company goes capture-private while the deals naming it stay where they
	// were.
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	middle := seedDealForCompany(t, e, "Deal with Mercator", pipeline, open, mid)
	zeta := seedDealForCompany(t, e, "Deal with Zeta", pipeline, open, last)

	// Admitted first: the reader must SEE all three deals, so what the sort
	// does below is the company's visibility and not the deal's.
	if got := dealsIn(ctx, t, e, "name"); len(got) != 3 {
		t.Fatalf("the reader sees %d deals, want 3 — this case would prove nothing about the ordering", len(got))
	}

	ascending := dealsIn(ctx, t, e, "company_id")
	assertIDOrder(t, ascending, []ids.UUID{middle, zeta, secret},
		"company ascending — the unreadable one in the tail, not first")

	// And the same position under the other direction. A name that really was
	// ordering the page would move to the other end.
	assertIDOrder(t, dealsIn(ctx, t, e, "-company_id"), []ids.UUID{zeta, middle, secret},
		"company descending — still the tail, because there is nothing to order by")
}

// A caller who may not read companies at all is ordered by no company.
//
// The row scope answers WHICH companies are visible and never whether this
// caller may read companies in the first place. A seat holding deal.read and no
// company.read would otherwise have its page arranged by names it is
// refused on every other surface.
func TestAReaderWithoutTheCompanyGrantOrdersTheDealsListByNothing(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)

	// Deal names and company names disagree, so a sort that still reached
	// display_name returns the page the other way round.
	zeta := seedDealForCompany(t, e, "Zeta deal", pipeline, open, e.SeedCompany(t, "Alma Werke", &e.Rep1))
	alma := seedDealForCompany(t, e, "Alma deal", pipeline, open, e.SeedCompany(t, "Zeta Holding", &e.Rep1))

	// Admitted first, as a reader who DOES hold the grant.
	assertIDOrder(t, dealsIn(e.Admin(), t, e, "company_id"), []ids.UUID{zeta, alma},
		"company ascending, for a reader who may read companies")

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})
	// Both rows in the tail under BOTH directions: the page falls back to its
	// tie-breaker and the order carries nothing about the companies.
	assertIDOrder(t, dealsIn(blind, t, e, "company_id"), []ids.UUID{alma, zeta},
		"company ascending, for a reader who may not read companies")
	assertIDOrder(t, dealsIn(blind, t, e, "-company_id"), []ids.UUID{alma, zeta},
		"company descending — the same order, because there is nothing to order by")
}

// seedDealForCompany creates a deal filed under one company, through the real
// writer.
func seedDealForCompany(
	t *testing.T, e *Env, name string, pipeline ids.PipelineID, stage ids.StageID, company ids.UUID,
) ids.UUID {
	t.Helper()
	companyID := ids.From[ids.CompanyKind](company)
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage,
		CompanyID: &companyID, OwnerID: userIDPtr(&e.Rep1),
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	return ids.UUID(d.Id)
}

// The page continues under a reference sort, including across the NULL tail.
//
// The keyset cursor carries the sort field's key and continues strictly past
// it, so an expression sort has to render the SAME expression in the ORDER BY
// and in the continuation — order by one and continue by another and a page
// boundary repeats rows or skips them. One row at a time is the setting that
// makes every boundary a boundary.
func TestAReferenceSortPagesWithoutRepeatingOrSkipping(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	hidden := e.SeedCompany(t, "Alma Werke", &e.Rep3)
	named := []ids.UUID{
		seedDealForCompany(t, e, "Deal with the hidden company", pipeline, open, hidden),
		seedDealForCompany(t, e, "Deal with Mercator", pipeline, open, e.SeedCompany(t, "Mercator", &e.Rep1)),
		seedDealForCompany(t, e, "Deal with Zeta", pipeline, open, e.SeedCompany(t, "Zeta Holding", &e.Rep1)),
	}
	// Two rows in the NULL tail — the hidden company and a deal filed under no
	// company at all — because the tail is where the continuation changes shape.
	bare := e.SeedDeal(t, "Deal with nobody", pipeline, open, &e.Rep1)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)

	spec, one := "company_id", 1
	var walked []ids.UUID
	var cursor *string
	for range len(named) + 2 {
		rows, page, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec, Limit: &one, Cursor: cursor})
		if err != nil {
			t.Fatalf("paging by company: %v", err)
		}
		for _, d := range rows {
			walked = append(walked, ids.UUID(d.Id))
		}
		if page.NextCursor == "" {
			break
		}
		next := page.NextCursor
		cursor = &next
	}

	// Every deal once, in the same order the unpaged read gives.
	assertIDOrder(t, walked, dealsIn(ctx, t, e, spec), "paged one at a time by company")
	if len(walked) != len(named)+1 {
		t.Fatalf("walked %d deals, want %d — a boundary repeated or skipped one", len(walked), len(named)+1)
	}
	if walked[len(walked)-1] != bare && walked[len(walked)-2] != bare {
		t.Errorf("the company-less deal landed at %v, not in the NULL tail", walked)
	}
}
