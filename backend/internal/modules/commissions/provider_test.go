// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The ledger is narrowed by partner, deal and status, each on the field the
// REST list fills for the same parameter.
func TestLedgerFiltersLandWhereTheRouteFillsThem(t *testing.T) {
	partner, deal := ids.NewV7(), ids.NewV7()
	var in ListInput
	if err := commissionListFilters.Apply(&in, map[string]string{
		"partner_company_id": partner.String(), "deal_id": deal.String(), "status": "payable",
	}); err != nil {
		t.Fatalf("applying every filter: %v", err)
	}
	if in.PartnerCompanyID == nil || in.PartnerCompanyID.UUID != partner {
		t.Errorf("partner_company_id landed as %v, want %s", in.PartnerCompanyID, partner)
	}
	if in.DealID == nil || in.DealID.UUID != deal {
		t.Errorf("deal_id landed as %v, want %s", in.DealID, deal)
	}
	if in.Status == nil || *in.Status != "payable" {
		t.Errorf("status landed as %v, want payable", in.Status)
	}
}

// The ledger offers its filters on the commission type only.
func TestOnlyTheCommissionTypeOffersTheLedgerFilters(t *testing.T) {
	p := &Provider{}
	if got, want := p.ListFilters(datasource.EntityCommission), []string{"deal_id", "partner_company_id", "status"}; !slices.Equal(got, want) {
		t.Errorf("commission offers %v, want %v", got, want)
	}
	if got := p.ListFilters(datasource.EntityDeal); got != nil {
		t.Errorf("deal is not the ledger's to list, yet it offers %v", got)
	}
}

// The ledger refuses before it reads. It refuses a type it does not hold, any
// text search, and a filter of the wrong shape.
func TestTheLedgerSeamRefusesBeforeItReads(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	var unsupported *datasource.UnsupportedEntityError
	if _, err := p.Read(ctx, datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}); !errors.As(err, &unsupported) {
		t.Errorf("reading a deal through the ledger answered %v, want an unsupported-type refusal", err)
	}
	if _, _, _, err := p.SearchEntity(ctx, datasource.EntityOffer, nil, 10, nil, nil); !errors.As(err, &unsupported) {
		t.Errorf("paging offers through the ledger answered %v, want an unsupported-type refusal", err)
	}

	text := "Northgate"
	_, _, _, err := p.SearchEntity(ctx, datasource.EntityCommission, &text, 10, nil, nil)
	var searched *httperr.DetailedError
	if !errors.As(err, &searched) || searched.Status != http.StatusUnprocessableEntity || searched.Fields[0].Field != "q" {
		t.Errorf("a text search of the ledger answered %v, want a validation refusal on q", err)
	}

	_, _, _, err = p.SearchEntity(ctx, datasource.EntityCommission, nil, 10, nil, map[string]string{"deal_id": "deal-7"})
	var shape *storekit.FilterError
	if !errors.As(err, &shape) || shape.Name != "deal_id" {
		t.Errorf("a malformed deal_id answered %v, want a refusal naming deal_id", err)
	}
}
