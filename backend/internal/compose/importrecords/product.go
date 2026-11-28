// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

// A product row as the deals store's rate-card inputs. A product is workspace
// data with no owner, tags or custom fields, so a file carries none of them. Its
// price is written in the major units of its currency, as a deal's amount is.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/importcells"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The columns of products.csv.
const (
	ProductName            = "name"
	ProductSKU             = "sku"
	ProductUnit            = "unit"
	ProductUnitPrice       = "unit_price"
	ProductCurrency        = "currency"
	ProductDefaultTaxRate  = "default_tax_rate"
	ProductTaxCategory     = "tax_category"
	ProductBillingModel    = "billing_model"
	ProductBillingInterval = "billing_interval_months"
	ProductActive          = "active"
)

// ProductTargets are the fields a product file may map.
var ProductTargets = []string{
	ProductName, ProductSKU, targetDescribed, ProductUnit, ProductUnitPrice, ProductCurrency,
	ProductDefaultTaxRate, ProductTaxCategory, ProductBillingModel, ProductBillingInterval, ProductActive,
}

// StoredColumns names the stored record's JSON field for each target a file
// spells differently, so an unedited cell compares equal to what is stored.
var StoredColumns = map[string]string{ProductUnitPrice: "unit_price_minor"}

type productRecord struct{ stores Stores }

func (r productRecord) Read(ctx context.Context, id ids.UUID) ([]byte, error) {
	product, err := r.stores.Products.GetProduct(ctx, ids.From[ids.ProductKind](id), storekit.LiveOnly)
	if err != nil {
		return nil, err
	}
	return encode(product)
}

func (r productRecord) Archived(ctx context.Context, id ids.UUID) (bool, error) {
	product, err := r.stores.Products.GetProduct(ctx, ids.From[ids.ProductKind](id), storekit.IncludeArchived)
	return err == nil && product.ArchivedAt != nil, err
}

// CheckCells reads a product row's price, tax rate, billing interval and active
// flag in the spelling the deals store takes them.
func (r productRecord) CheckCells(fields map[string]string, typed map[string]any) (map[string]any, []*CellProblem) {
	if typed == nil {
		typed = map[string]any{}
	}
	var problems []*CellProblem
	if problem := CheckMoney(fields, typed, ProductUnitPrice, ProductCurrency); problem != nil {
		problems = append(problems, problem)
	}
	if text := strings.TrimSpace(fields[ProductDefaultTaxRate]); text != "" {
		rate, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
		if err != nil {
			problems = append(problems, ProblemOf(ProductDefaultTaxRate, fields[ProductDefaultTaxRate], migration.Why(migration.ReasonNotANumber, "value", text)))
		} else {
			typed[ProductDefaultTaxRate] = rate
		}
	}
	if text := strings.TrimSpace(fields[ProductBillingInterval]); text != "" {
		months, err := strconv.Atoi(text)
		if err != nil {
			problems = append(problems, ProblemOf(ProductBillingInterval, fields[ProductBillingInterval], migration.Why(migration.ReasonNotANumber, "value", text)))
		} else {
			typed[ProductBillingInterval] = months
		}
	}
	if text := strings.TrimSpace(fields[ProductActive]); text != "" {
		active, why := importcells.YesNo(text)
		if why.Code != "" {
			problems = append(problems, ProblemOf(ProductActive, fields[ProductActive], why))
		} else {
			typed[ProductActive] = active
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return typed, nil
}

// RefuseNew refuses a row with no name.
func (r productRecord) RefuseNew(fields map[string]string, _ map[string]any) migration.Reason {
	if Text(fields, ProductName) == nil {
		return migration.Why(migration.ReasonProductUnnamed)
	}
	return migration.Reason{}
}

// birthOf is what a row asks the deals store to create. The test run and the
// commit both build it here.
func (r productRecord) birthOf(fields map[string]string, typed map[string]any) deals.CreateProductInput {
	in := deals.CreateProductInput{
		SKU:         Text(fields, ProductSKU),
		Description: Text(fields, targetDescribed),
		Unit:        Text(fields, ProductUnit),
		TaxCategory: Text(fields, ProductTaxCategory),
		Source:      string(provenance.RecordSourceImport),
	}
	if name := Text(fields, ProductName); name != nil {
		in.Name = *name
	}
	in.UnitPriceMinor, _ = typed[ProductUnitPrice].(int64)
	in.Currency, _ = typed[ProductCurrency].(string)
	if rate, ok := typed[ProductDefaultTaxRate].(float64); ok {
		in.DefaultTaxRate = &rate
	}
	if active, ok := typed[ProductActive].(bool); ok {
		in.Active = &active
	}
	in.BillingModel = Text(fields, ProductBillingModel)
	if months, ok := typed[ProductBillingInterval].(int); ok {
		in.BillingIntervalMonths = &months
	}
	return in
}

func (r productRecord) PredictCreate(ctx context.Context, fields map[string]string, typed map[string]any) (migration.Reason, error) {
	_, err := r.stores.Products.PreviewProductCreation(ctx, r.birthOf(fields, typed))
	if why, refused := r.Refusal(err, fields); refused {
		return why, nil
	}
	return migration.Reason{}, err
}

func (r productRecord) Create(ctx context.Context, land Landing, row migration.Row, typed map[string]any, _ string) (migration.EnsureResult, error) {
	fields := TextFields(row.Fields)
	in := r.birthOf(fields, typed)
	err := land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		product, err := r.stores.Products.CreateProductTx(ctx, tx, in)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating product %s: %w", row.ExternalID, err)
		}
		return ids.UUID(product.Id), nil
	})
	if why, refused := r.Refusal(err, fields); refused {
		return SkippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// updateFrom builds the update for the cells that differ from the stored
// product. The billing model and its interval are one fact in two columns.
// A change to either restates both, with the unchanged half from the stored product.
func (r productRecord) updateFrom(stored crmcontracts.Product, changed map[string]string, typed map[string]any) deals.UpdateProductInput {
	in := deals.UpdateProductInput{
		Name:        Text(changed, ProductName),
		SKU:         Text(changed, ProductSKU),
		Description: Text(changed, targetDescribed),
		Unit:        Text(changed, ProductUnit),
		TaxCategory: Text(changed, ProductTaxCategory),
	}
	if _, given := changed[ProductUnitPrice]; given {
		if minor, ok := typed[ProductUnitPrice].(int64); ok {
			in.UnitPriceMinor = &minor
		}
	}
	if _, given := changed[ProductCurrency]; given {
		if currency, ok := typed[ProductCurrency].(string); ok {
			in.Currency = &currency
		}
	}
	if rate, ok := typed[ProductDefaultTaxRate].(float64); ok && changed[ProductDefaultTaxRate] != "" {
		in.DefaultTaxRate = &rate
	}
	if active, ok := typed[ProductActive].(bool); ok && changed[ProductActive] != "" {
		in.Active = &active
	}
	_, modelMoved := changed[ProductBillingModel]
	_, intervalMoved := changed[ProductBillingInterval]
	if modelMoved || intervalMoved {
		in.Classified = true
		in.BillingModel, in.BillingIntervalMonths = billingAfter(stored, changed, typed)
	}
	return in
}

// billingAfter is the classification a product holds once the file's cells are
// applied to it. A product moved to one-off with no interval in the file loses
// its stored interval, since a one-off price has none.
func billingAfter(stored crmcontracts.Product, changed map[string]string, typed map[string]any) (*string, *int) {
	_, modelMoved := changed[ProductBillingModel]
	_, intervalMoved := changed[ProductBillingInterval]
	var model *string
	if stored.BillingModel != nil {
		model = new(string(*stored.BillingModel))
	}
	if modelMoved {
		model = Text(changed, ProductBillingModel)
	}
	var months *int
	if stored.BillingIntervalMonths != nil {
		months = new(int(*stored.BillingIntervalMonths))
	}
	switch {
	case intervalMoved:
		months = nil
		if value, ok := typed[ProductBillingInterval].(int); ok {
			months = &value
		}
	case model != nil && *model == deals.BillingOneTime:
		months = nil
	}
	return model, months
}

// RefuseChange refuses a row whose cells, laid over the stored product, the
// deals store would refuse: a rate or a billing shape outside its range.
func (r productRecord) RefuseChange(current []byte, changed map[string]string, typed map[string]any) migration.Reason {
	var stored crmcontracts.Product
	if err := json.Unmarshal(current, &stored); err != nil {
		return migration.Why(migration.ReasonValueUnreadable, "value", "product")
	}
	why, _ := r.Refusal(deals.RefuseProductUpdate(stored, r.updateFrom(stored, changed, typed)), changed)
	return why
}

func (r productRecord) Update(ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any, current []byte, ifVersion *int64) error {
	var stored crmcontracts.Product
	if err := json.Unmarshal(current, &stored); err != nil {
		return fmt.Errorf("import: reading the stored product: %w", err)
	}
	in := r.updateFrom(stored, changed, typed)
	in.IfVersion = ifVersion
	_, err := r.stores.Products.UpdateProduct(ctx, ids.From[ids.ProductKind](id), in)
	return err
}

func (r productRecord) Undo(ctx context.Context, id ids.UUID, importedAt time.Time) error {
	_, err := r.stores.Products.ArchiveProductAt(ctx, ids.From[ids.ProductKind](id), nil, storekit.NotTouchedByHumanSince(importedAt))
	return err
}

// storeTargets maps the store's own field names to the targets of the file.
var storeTargets = map[string]string{"unit_price_minor": ProductUnitPrice}

// Refusal answers the deals store's refusals of one product row as the reason
// the report shows. A SKU another live product holds is the one conflict a
// product write raises.
func (r productRecord) Refusal(err error, fields map[string]string) (migration.Reason, bool) {
	if err == nil {
		return migration.Reason{}, false
	}
	if errors.Is(err, apperrors.ErrConflict) {
		return migration.Why(migration.ReasonProductSKUHeld, "value", fields[ProductSKU]), true
	}
	return storeFieldRefusal(err, fields, storeTargets)
}

func (r productRecord) Lock(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	return migration.ObjectProduct, r.stores.Products.LockProductLive(ctx, tx, ids.From[ids.ProductKind](id))
}

// ProductBirth is the product a row would create, for the review an agent's
// import waits on.
func ProductBirth(fields map[string]string, typed map[string]any) deals.CreateProductInput {
	return productRecord{}.birthOf(fields, typed)
}
