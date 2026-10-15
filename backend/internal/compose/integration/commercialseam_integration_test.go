// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// commercialRep reads deals and what hangs off them, and writes offers, on its
// own team's rows.
var commercialRep = principal.Permissions{
	Objects: map[string]principal.ObjectGrant{
		"deal": {Read: true, Update: true}, "offer": {Read: true, Update: true},
		"commission": {Read: true}, "deal_room": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// The commission ledger and an offer's writes take their deal's scope on the
// record seam the MCP tools run through, as on REST. A deal is readable by
// every seat, so its offers are too. Who may change one, or read a commission,
// is the deal's owner and team.
func TestTheCommercialRecordsTakeTheirDealsScopeOnTheSeam(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)
	admin := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}, "offer": {Create: true, Read: true}},
		RowScope: principal.RowScopeAll,
	})
	won, err := e.Deals.GetDeal(admin, fx.deal, storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("reading the fixture's deal: %v", err)
	}
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Offer deal", ids.From[ids.PipelineKind](ids.UUID(*won.PipelineId)), fx.open, &e.Rep1))
	offer, err := e.Deals.CreateOffer(admin, deal, deals.CreateOfferInput{Currency: "EUR", Source: "manual"})
	if err != nil {
		t.Fatalf("seeding an offer: %v", err)
	}

	seam := compose.NewProviderFor(e.DB())
	onTheTeam := e.As(e.Rep1, []ids.UUID{e.Team1}, commercialRep)
	elsewhere := e.As(e.Rep3, []ids.UUID{e.Team2}, commercialRep)

	ledger := datasource.SearchQuery{
		EntityTypes: []datasource.EntityType{datasource.EntityCommission}, Filters: map[string]string{"deal_id": fx.deal.String()},
	}
	entry := commissionOn(t, seam, onTheTeam, ledger)
	if _, err := seam.Read(elsewhere, datasource.EntityRef{Type: datasource.EntityCommission, ID: entry}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a rep on another team read a commission on a deal they do not own: %v, want not found", err)
	}
	if page, err := seam.Search(elsewhere, ledger); err != nil || len(page.Records) != 0 {
		t.Errorf("a rep on another team lists %d commission(s) on a deal they do not own (%v)", len(page.Records), err)
	}

	patch := func(ctx context.Context) error {
		_, err := seam.Update(ctx, datasource.UpdateInput{
			Ref: datasource.EntityRef{Type: datasource.EntityOffer, ID: ids.UUID(offer.Id)}, Patch: map[string]any{"intro_text": "Hello"},
		})
		return err
	}
	if err := patch(onTheTeam); err != nil {
		t.Errorf("the deal's own rep could not change its offer: %v", err)
	}
	if err := patch(elsewhere); err == nil {
		t.Error("a rep on another team changed an offer on a deal they may not write")
	}
}

// commissionOn is the one ledger row the fixture accrued on its deal.
func commissionOn(t *testing.T, seam *compose.Provider, ctx context.Context, ledger datasource.SearchQuery) ids.UUID {
	t.Helper()
	page, err := seam.Search(ctx, ledger)
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("the seam lists %d commission(s) on the won deal (%v), want the one accrued", len(page.Records), err)
	}
	return page.Records[0].Ref.ID
}
