// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Provider answers the record seam's reads of the commission ledger, through
// the store reads the REST handlers run. They apply the same row scope, and
// the same field mask that drops a row whose figures the reader may not see.
type Provider struct {
	store *Store
}

// NewProvider builds the ledger's read provider over a workspace-bound handle.
func NewProvider(db *database.DB) *Provider {
	return &Provider{store: NewStore(db)}
}

const (
	filterPartnerCompanyID = "partner_company_id"
	filterDealID           = "deal_id"
	filterStatus           = "status"
)

var commissionListFilters = storekit.FilterSet[ListInput]{
	filterPartnerCompanyID: storekit.FilterID(func(in *ListInput, id *ids.CompanyID) { in.PartnerCompanyID = id }),
	filterDealID:           storekit.FilterID(func(in *ListInput, id *ids.DealID) { in.DealID = id }),
	filterStatus:           storekit.FilterWord(func(in *ListInput, v *string) { in.Status = v }),
}

// ListFilters names what the ledger can be narrowed by on the seam.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	if t != datasource.EntityCommission {
		return nil
	}
	return commissionListFilters.Names()
}

// Read answers one ledger row.
func (p *Provider) Read(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	if r.Type != datasource.EntityCommission {
		return datasource.Record{}, &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	v, err := p.store.GetCommissionEntry(ctx, ids.CommissionEntryID{UUID: r.ID})
	if err != nil {
		return datasource.Record{}, err
	}
	return datasource.NewRecord(r, v, v.Version)
}

// SearchEntity pages the ledger. It has no text to search.
func (p *Provider) SearchEntity(ctx context.Context, t datasource.EntityType, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if t != datasource.EntityCommission {
		return nil, "", false, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	if text != nil {
		return nil, "", false, httperr.Validation("q", "unsupported",
			"the commission ledger has no text to search: list it, narrowed by partner, deal or status")
	}
	in := ListInput{Limit: &limit, Cursor: cursor}
	if err := commissionListFilters.Apply(&in, filters); err != nil {
		return nil, "", false, err
	}
	page, err := p.store.List(ctx, in)
	if err != nil {
		return nil, "", false, err
	}
	records := make([]datasource.Record, 0, len(page.Data))
	for _, v := range page.Data {
		rec, err := datasource.NewRecord(datasource.EntityRef{Type: t, ID: ids.UUID(v.Id)}, v, v.Version)
		if err != nil {
			return nil, "", false, err
		}
		records = append(records, rec)
	}
	next := ""
	if page.Page.NextCursor != nil {
		next = *page.Page.NextCursor
	}
	return records, next, page.Page.HasMore, nil
}
