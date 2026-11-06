// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The deal columns a line write moves besides the money.
const (
	commercialModeColumn     = "commercial_mode"
	commercialRevisionColumn = "commercial_revision"
)

// The columns a deal's lines, offers and product split share.
const (
	dealIDColumn        = "deal_id"
	productIDColumn     = "product_id"
	sourceColumn        = "source"
	componentKindColumn = "component_kind"
)

// settleDealLines derives the deal's value from its lines, in the line write's
// transaction. The deal row goes through the patch seam, so its forecast move
// is recorded. Then come the product split, the audit row and deal.updated. A
// failure anywhere rolls the line write back with it.
func (s *Store) settleDealLines(ctx context.Context, tx pgx.Tx, open openedDealLines, change lineChange) error {
	lines, err := readDealLineRows(ctx, tx, open.DealID)
	if err != nil {
		return err
	}
	_, figures, err := s.dealLineFigures(ctx, tx, lines)
	if err != nil {
		return err
	}
	p := storekit.NewPatch()
	if open.Head.Mode != modeLines {
		p.Set(commercialModeColumn, open.Head.Mode, modeLines)
	}
	p.Set(commercialRevisionColumn, open.Head.Revision, open.Head.Revision+1)
	setLineMoney(p, open.Head, figures, commercialCurrency(open.Head, lines))
	if open.Head.ArrSourceOfferID != nil {
		// The lines state the recurring value now, so no offer does.
		p.Set(arrSourceField, *open.Head.ArrSourceOfferID, nil)
	}
	if err := applyDealLineValueLocked(ctx, tx, p, open.Lock); err != nil {
		return fmt.Errorf("apply the deal's line value: %w", err)
	}
	before, err := readDealProducts(ctx, tx, open.DealID)
	if err != nil {
		return err
	}
	products, err := projectDealLineProducts(ctx, tx, open, lines, figures)
	if err != nil {
		return err
	}
	return recordDealLineWrite(ctx, tx, open, p, productMove{Before: before, After: products}, change.evidence(), change.event())
}

// setLineMoney writes the deal's amount, recurring value and currency from
// its lines. Each figure is null while its own inputs are incomplete, and the
// currency stands only beside a figure, as deal_money_currency_pair holds.
func setLineMoney(p *storekit.Patch, head dealCommercialHead, figures DealCommercialFigures, linesCurrency *string) {
	setMinor := func(column string, before, after *int64) {
		if equalMinor(before, after) {
			return
		}
		if after == nil {
			p.Set(column, before, nil)
			return
		}
		p.Set(column, before, *after)
	}
	setMinor(amountField, head.AmountMinor, figures.CommittedNetMinor)
	setMinor(arrField, head.ArrMinor, figures.ArrMinor)
	var currency *string
	if figures.CommittedNetMinor != nil || figures.ArrMinor != nil {
		currency = linesCurrency
	}
	switch {
	case currency == nil && head.Currency != nil:
		p.Set(currencyField, *head.Currency, nil)
	case currency != nil && (head.Currency == nil || *head.Currency != *currency):
		p.Set(currencyField, head.Currency, *currency)
	}
}

func equalMinor(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// leaveLinesMode returns a deal with no line to a typed estimate.
func (s *Store) leaveLinesMode(ctx context.Context, tx pgx.Tx, open openedDealLines) error {
	p := storekit.NewPatch()
	p.Set(commercialModeColumn, open.Head.Mode, modeEstimate)
	p.Set(commercialRevisionColumn, open.Head.Revision, open.Head.Revision+1)
	if err := applyDealLineValueLocked(ctx, tx, p, open.Lock); err != nil {
		return fmt.Errorf("return the deal to an estimate: %w", err)
	}
	estimate := map[string]any{"use_estimate": true}
	return recordDealLineWrite(ctx, tx, open, p, productMove{}, estimate, estimate)
}

// projectDealLineProducts rewrites the deal's product split from its lines:
// one `line` row per product, summing every line that names it. A product
// with an incomplete line has no amount and keeps its known part. Custom items
// name no product and stay out of the split.
func projectDealLineProducts(ctx context.Context, tx pgx.Tx, open openedDealLines, lines []dealLineRow, figures DealCommercialFigures) ([]crmcontracts.DealProduct, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM deal_product WHERE deal_id = $1`, open.DealID); err != nil {
		return nil, fmt.Errorf("clear the deal's products: %w", err)
	}
	type productSum struct {
		known    int64
		complete bool
		kind     *string
		currency string
	}
	sums := map[string]*productSum{}
	var order []string
	for i, line := range lines {
		if line.ProductID == nil {
			continue
		}
		key := line.ProductID.String()
		sum, seen := sums[key]
		if !seen {
			sum = &productSum{complete: true, kind: line.ComponentKind, currency: line.Currency}
			sums[key] = sum
			order = append(order, key)
		}
		committed := figures.Lines[i].CommittedNetMinor
		if committed == nil {
			sum.complete = false
			continue
		}
		sum.known += *committed
	}
	revision := open.Head.Revision + 1
	for _, key := range order {
		sum := sums[key]
		var amount *int64
		if sum.complete {
			amount = &sum.known
		}
		cols, holders, args := columnValues(
			columnValue{dealIDColumn, open.DealID}, columnValue{productIDColumn, key}, columnValue{sourceColumn, "line"},
			columnValue{amountField, amount}, columnValue{"known_amount_minor", sum.known},
			columnValue{currencyField, sum.currency}, columnValue{componentKindColumn, sum.kind},
			columnValue{"commercial_revision", revision})
		if _, err := tx.Exec(ctx, `INSERT INTO deal_product (`+cols+`) VALUES (`+holders+`)`, args...); err != nil {
			return nil, fmt.Errorf("write a line product: %w", err)
		}
	}
	return readDealProducts(ctx, tx, open.DealID)
}

// lineChange is the line a write added, changed or removed, as it stood on
// either side. The audit keeps both images; the event names only the line.
type lineChange struct {
	Kind          string
	Before, After *dealLineRow
}

func (c lineChange) id() ids.UUID {
	if c.After != nil {
		return c.After.ID
	}
	return c.Before.ID
}

// evidence is what the audit row records about the line itself.
//
//craft:ignore naked-any the evidence is the jsonb object the audit seam stores
func (c lineChange) evidence() map[string]any {
	evidence := lineFieldEvidence(c.id(), dealLineImage(c.Before), dealLineImage(c.After))
	evidence["line_change"] = c.Kind
	return evidence
}

//craft:ignore naked-any an event's changed fields are a free-form object
func (c lineChange) event() map[string]any {
	return map[string]any{"line_change": c.Kind, lineIDKey: c.id()}
}

// dealLineImage is one line's fields as the audit records them, nil for a
// line that is not there.
//
//craft:ignore naked-any an audit image carries whatever SQL types its columns hold
func dealLineImage(line *dealLineRow) map[string]any {
	if line == nil {
		return nil
	}
	return map[string]any{
		productIDColumn: line.ProductID, descriptionColumn: line.Description, lineUnitColumn: line.Unit,
		lineQuantityColumn: line.Quantity, lineUnitPriceColumn: line.UnitPriceMinor, currencyField: line.Currency,
		lineDiscountColumn: line.DiscountPct, lineTaxRateColumn: line.TaxRate, taxCategoryField: line.TaxCategory,
		lineBillingModelColumn: line.BillingModel, billingIntervalColumn: line.IntervalMonths, intervalCountField: line.IntervalCount,
	}
}

// productMove is the deal's product split before and after a line write.
type productMove struct{ Before, After []crmcontracts.DealProduct }

// recordDealLineWrite audits the deal's move and publishes it as one
// deal.updated. The event carries the commercial revision on every write, so
// a subscriber can tell an equal-total edit from no edit.
func recordDealLineWrite(ctx context.Context, tx pgx.Tx, open openedDealLines, p *storekit.Patch, products productMove, evidence, event map[string]any) error {
	before, after := maps.Clone(p.Before()), maps.Clone(p.After())
	if products.Before != nil || products.After != nil {
		before[productsField], after[productsField] = allocationImage(products.Before), allocationImage(products.After)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "deal", open.DealID.UUID, before, after, evidence)
	if err != nil {
		return fmt.Errorf("audit the deal's lines: %w", err)
	}
	changed := maps.Clone(after)
	maps.Copy(changed, event)
	updated := crmcontracts.PublicEventDealUpdated{ChangedFields: changed}
	updated.Imported, updated.ImportRunId = storekit.ImportMark(ctx)
	if err := storekit.EmitEvent(ctx, tx, auditID, open.DealID.UUID, updated); err != nil {
		return fmt.Errorf("emit deal.updated for the deal's lines: %w", err)
	}
	return nil
}
