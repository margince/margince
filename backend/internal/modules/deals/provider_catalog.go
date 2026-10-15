// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"encoding/json"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The catalog half of the deals provider: products and offer templates. It
// runs the store methods and request rules the REST handlers run, so a passport
// meets one catalog on either door.

const (
	filterActive = "active"
	filterLocale = "locale"
)

var productListFilters = storekit.FilterSet[ListProductsInput]{
	filterActive:          storekit.FilterFlag(func(in *ListProductsInput, v *bool) { in.Active = v }),
	filterIncludeArchived: storekit.FilterFlag(func(in *ListProductsInput, v *bool) { in.IncludeArchived = *v }),
	filterSort:            storekit.FilterWord(func(in *ListProductsInput, v *string) { in.Sort = v }),
}

var offerTemplateListFilters = storekit.FilterSet[ListOfferTemplatesInput]{
	filterLocale:          storekit.FilterWord(func(in *ListOfferTemplatesInput, v *string) { in.Locale = v }),
	filterIncludeArchived: storekit.FilterFlag(func(in *ListOfferTemplatesInput, v *bool) { in.IncludeArchived = *v }),
	filterSort:            storekit.FilterWord(func(in *ListOfferTemplatesInput, v *string) { in.Sort = v }),
}

func (p *Provider) readCatalog(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	switch r.Type {
	case datasource.EntityProduct:
		v, err := p.store.GetProduct(ctx, ids.From[ids.ProductKind](r.ID), storekit.IncludeArchived)
		if err != nil {
			return datasource.Record{}, err
		}
		return datasource.NewRecord(r, v, v.Version)
	default:
		v, err := p.store.GetOfferTemplate(ctx, ids.From[ids.OfferTemplateKind](r.ID), storekit.IncludeArchived)
		if err != nil {
			return datasource.Record{}, err
		}
		return datasource.NewRecord(r, v, v.Version)
	}
}

func (p *Provider) searchCatalog(ctx context.Context, t datasource.EntityType, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if t == datasource.EntityProduct {
		in := ListProductsInput{Query: text, Limit: &limit, Cursor: cursor}
		if err := productListFilters.Apply(&in, filters); err != nil {
			return nil, "", false, err
		}
		rows, page, err := p.store.ListProducts(ctx, in)
		if err != nil {
			return nil, "", false, err
		}
		return catalogRecords(t, rows, page, func(v crmcontracts.Product) (openapi_types.UUID, *int64) { return v.Id, v.Version })
	}
	if text != nil {
		return nil, "", false, httperr.Validation("q", "unsupported",
			"offer templates have no text to search: list them, narrowed by locale")
	}
	in := ListOfferTemplatesInput{Limit: &limit, Cursor: cursor}
	if err := offerTemplateListFilters.Apply(&in, filters); err != nil {
		return nil, "", false, err
	}
	rows, page, err := p.store.ListOfferTemplates(ctx, in)
	if err != nil {
		return nil, "", false, err
	}
	return catalogRecords(t, rows, page, func(v crmcontracts.OfferTemplate) (openapi_types.UUID, *int64) { return v.Id, v.Version })
}

func catalogRecords[V any](t datasource.EntityType, rows []V, page storekit.Page, key func(V) (openapi_types.UUID, *int64)) ([]datasource.Record, string, bool, error) {
	records := make([]datasource.Record, 0, len(rows))
	for _, v := range rows {
		id, version := key(v)
		rec, err := datasource.NewRecord(ref(t, id), v, version)
		if err != nil {
			return nil, "", false, err
		}
		records = append(records, rec)
	}
	return records, page.NextCursor, page.HasMore, nil
}

func (p *Provider) createCatalog(ctx context.Context, t datasource.EntityType, raw json.RawMessage, source string) (datasource.EntityRef, error) {
	if t == datasource.EntityProduct {
		var req crmcontracts.CreateProductRequest
		if err := datasource.StrictDecode(raw, &req); err != nil {
			return datasource.EntityRef{}, err
		}
		req.Source = source
		in, err := productCreateInput(req, statesAValue(raw, "unit_price_minor"))
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.CreateProduct(ctx, in)
		return ref(t, v.Id), err
	}
	var req crmcontracts.CreateOfferTemplateRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	in, err := offerTemplateCreateInput(req)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	v, err := p.store.CreateOfferTemplate(ctx, in)
	return ref(t, v.Id), err
}

func (p *Provider) updateCatalog(ctx context.Context, in datasource.UpdateInput, raw json.RawMessage) (datasource.EntityRef, error) {
	if in.Ref.Type == datasource.EntityProduct {
		var req crmcontracts.UpdateProductRequest
		if err := datasource.StrictDecode(raw, &req); err != nil {
			return datasource.EntityRef{}, err
		}
		update, err := productUpdateInput(req, in.Clear, in.IfVersion)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateProduct(ctx, ids.From[ids.ProductKind](in.Ref.ID), update)
		return ref(in.Ref.Type, v.Id), err
	}
	var req crmcontracts.UpdateOfferTemplateRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	update, err := offerTemplateUpdateInput(req, in.IfVersion)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	v, err := p.store.UpdateOfferTemplate(ctx, ids.From[ids.OfferTemplateKind](in.Ref.ID), update)
	return ref(in.Ref.Type, v.Id), err
}

// refuseArchiveCatalog is the stage-time probe: the grant, then the live row.
func (p *Provider) refuseArchiveCatalog(ctx context.Context, r datasource.EntityRef) error {
	if err := auth.Require(ctx, string(r.Type), principal.ActionDelete); err != nil {
		return err
	}
	_, err := p.readCatalogLive(ctx, r)
	return err
}

func (p *Provider) readCatalogLive(ctx context.Context, r datasource.EntityRef) (openapi_types.UUID, error) {
	if r.Type == datasource.EntityProduct {
		v, err := p.store.GetProduct(ctx, ids.From[ids.ProductKind](r.ID), storekit.LiveOnly)
		return v.Id, err
	}
	v, err := p.store.GetOfferTemplate(ctx, ids.From[ids.OfferTemplateKind](r.ID), storekit.LiveOnly)
	return v.Id, err
}

func (p *Provider) archiveCatalog(ctx context.Context, r datasource.EntityRef) (datasource.EntityRef, error) {
	if r.Type == datasource.EntityProduct {
		v, err := p.store.ArchiveProduct(ctx, ids.From[ids.ProductKind](r.ID))
		return ref(r.Type, v.Id), err
	}
	v, err := p.store.ArchiveOfferTemplate(ctx, ids.From[ids.OfferTemplateKind](r.ID))
	return ref(r.Type, v.Id), err
}

// statesAValue reports whether a body names key with a value other than null.
func statesAValue(raw json.RawMessage, key string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	value, ok := fields[key]
	return ok && string(value) != "null"
}
