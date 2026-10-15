// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"encoding/json"
	"log/slog"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The offer half of the deals provider. It runs the store methods and request
// rules the REST handlers run. An offer reads, lists and changes alike on
// either door, under its deal's row scope.

const filterOfferDealID = "deal_id"

// offerListArgs is a list of one deal's offers: the route names the deal in
// its path, the seam as a filter.
type offerListArgs struct {
	dealID *ids.DealID
	in     ListDealOffersInput
}

var offerListFilters = storekit.FilterSet[offerListArgs]{
	filterOfferDealID: storekit.FilterID(func(a *offerListArgs, id *ids.DealID) { a.dealID = id }),
	filterStatus:      storekit.FilterWord(func(a *offerListArgs, v *string) { a.in.Status = v }),
}

// RetiredRendering removes a rendering an offer change retired. Both doors
// reclaim it. The row no longer names it, so a failed delete leaves an inert
// orphan, logged rather than failing the edit.
type RetiredRendering func(ctx context.Context, ref string) error

// WithRetiredRendering lets an offer update reclaim the rendering it retired.
func (p *Provider) WithRetiredRendering(reclaim RetiredRendering) *Provider {
	p.reclaim = reclaim
	return p
}

func (p *Provider) readOffer(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	v, err := p.store.GetOffer(ctx, ids.From[ids.OfferKind](r.ID), storekit.IncludeArchived)
	if err != nil {
		return datasource.Record{}, err
	}
	return datasource.NewRecord(r, v, v.Version)
}

func (p *Provider) searchOffers(ctx context.Context, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if text != nil {
		return nil, "", false, httperr.Validation("q", "unsupported", "offers have no text to search: list one deal's, by deal_id")
	}
	args := offerListArgs{in: ListDealOffersInput{Limit: &limit, Cursor: cursor}}
	if err := offerListFilters.Apply(&args, filters); err != nil {
		return nil, "", false, err
	}
	if args.dealID == nil {
		return nil, "", false, httperr.Validation(filterOfferDealID, "required", "offers are listed one deal at a time: name its deal_id")
	}
	rows, page, err := p.store.ListDealOffers(ctx, *args.dealID, args.in)
	if err != nil {
		return nil, "", false, err
	}
	return catalogRecords(datasource.EntityOffer, rows, page, func(v crmcontracts.Offer) (openapi_types.UUID, *int64) {
		return v.Id, v.Version
	})
}

// createOffer makes an offer on the deal its fields name, as POST
// /deals/{id}/offers makes one on the deal its path names.
func (p *Provider) createOffer(ctx context.Context, raw json.RawMessage, source string) (datasource.EntityRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return datasource.EntityRef{}, httperr.Validation("fields", "malformed_json", err.Error())
	}
	var dealID ids.UUID
	if err := json.Unmarshal(fields[filterOfferDealID], &dealID); err != nil || dealID.IsZero() {
		return datasource.EntityRef{}, httperr.Validation(filterOfferDealID, "required", "an offer is made on a deal: name its deal_id")
	}
	delete(fields, filterOfferDealID)
	fields["source"], _ = json.Marshal(source)
	body, err := json.Marshal(fields)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	in, err := offerCreateInput(body)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	v, err := p.store.CreateOffer(ctx, ids.From[ids.DealKind](dealID), in)
	return datasource.EntityRef{Type: datasource.EntityOffer, ID: ids.UUID(v.Id)}, err
}

func (p *Provider) updateOffer(ctx context.Context, in datasource.UpdateInput, raw json.RawMessage) (datasource.EntityRef, error) {
	var req crmcontracts.UpdateOfferRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	update, err := offerUpdateInput(req, in.Clear, in.IfVersion)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	offerID := ids.From[ids.OfferKind](in.Ref.ID)
	v, retired, err := p.store.UpdateOffer(ctx, offerID, update)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	if retired != nil && p.reclaim != nil {
		if delErr := p.reclaim(ctx, *retired); delErr != nil {
			slog.WarnContext(ctx, "reclaiming the rendering a buyer change retired",
				"offer", offerID.String(), "ref", *retired, "err", delErr)
		}
	}
	return datasource.EntityRef{Type: datasource.EntityOffer, ID: ids.UUID(v.Id)}, nil
}

func (p *Provider) refuseArchiveOffer(ctx context.Context, r datasource.EntityRef) error {
	return p.store.RefuseArchiveOffer(ctx, ids.From[ids.OfferKind](r.ID))
}

func (p *Provider) archiveOffer(ctx context.Context, r datasource.EntityRef) (datasource.EntityRef, error) {
	v, err := p.store.ArchiveOffer(ctx, ids.From[ids.OfferKind](r.ID))
	return datasource.EntityRef{Type: datasource.EntityOffer, ID: ids.UUID(v.Id)}, err
}

// AddOfferLine adds one line item, under the same rules POST
// /offers/{id}/line-items applies, and answers the offer it changed.
func (p *Provider) AddOfferLine(ctx context.Context, offerID ids.UUID, raw json.RawMessage) (crmcontracts.Offer, error) {
	line, err := addLineInput(raw)
	if err != nil {
		return crmcontracts.Offer{}, err
	}
	return p.store.AddOfferLineItem(ctx, ids.From[ids.OfferKind](offerID), line)
}

// UpdateOfferLine patches one line item, as PATCH
// /offers/{id}/line-items/{lineItemId} does.
func (p *Provider) UpdateOfferLine(ctx context.Context, offerID, lineID ids.UUID, raw json.RawMessage) (crmcontracts.Offer, error) {
	in, err := updateLineInput(raw)
	if err != nil {
		return crmcontracts.Offer{}, err
	}
	return p.store.UpdateOfferLineItem(ctx, ids.From[ids.OfferKind](offerID), lineID, in)
}

// RemoveOfferLine removes one line item, as the DELETE route does.
func (p *Provider) RemoveOfferLine(ctx context.Context, offerID, lineID ids.UUID) (crmcontracts.Offer, error) {
	return p.store.RemoveOfferLineItem(ctx, ids.From[ids.OfferKind](offerID), lineID)
}
