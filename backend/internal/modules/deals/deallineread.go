// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The deal's commercial modes, as the commercial_mode column spells them.
const (
	modeEstimate = "estimate"
	modeLines    = "lines"
	modeLegacy   = "legacy"
)

// dealLineRow is one deal_line_item row. Quantity, price and the typed tax
// rate are exact decimal strings or minor units, nil where nobody knows them.
type dealLineRow struct {
	ID             ids.UUID
	Position       int
	ProductID      *ids.UUID
	CatalogName    *string
	SKU            *string
	Description    string
	Unit           string
	Quantity       *string
	UnitPriceMinor *int64
	Currency       string
	DiscountPct    string
	TaxRate        *string
	TaxCategory    *string
	BillingModel   *string
	IntervalMonths *int
	IntervalCount  *int
	ComponentKind  *string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// dealCommercialHead is what a line write reads off the deal row before it
// decides anything, under the deal's row lock.
type dealCommercialHead struct {
	Mode             string
	Revision         int64
	Status           string
	Currency         *string
	AmountMinor      *int64
	ArrMinor         *int64
	ArrSourceOfferID *ids.UUID
}

func readDealCommercialHead(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (dealCommercialHead, error) {
	var h dealCommercialHead
	err := tx.QueryRow(ctx,
		`SELECT commercial_mode, commercial_revision, status, currency, amount_minor, expected_arr_minor, arr_source_offer_id
		   FROM deal WHERE id = $1`, dealID).
		Scan(&h.Mode, &h.Revision, &h.Status, &h.Currency, &h.AmountMinor, &h.ArrMinor, &h.ArrSourceOfferID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dealCommercialHead{}, apperrors.ErrNotFound
	}
	if err != nil {
		return dealCommercialHead{}, fmt.Errorf("read the deal's commercial state: %w", err)
	}
	return h, nil
}

const dealLineColumns = `id, position, product_id, catalog_name, sku, description, unit, quantity::text,
	unit_price_minor, currency, discount_pct::text, tax_rate::text, tax_category, billing_model,
	billing_interval_months, interval_count, component_kind, version, created_at, updated_at`

func scanDealLine(row pgx.Row) (dealLineRow, error) {
	var l dealLineRow
	err := row.Scan(&l.ID, &l.Position, &l.ProductID, &l.CatalogName, &l.SKU, &l.Description, &l.Unit, &l.Quantity,
		&l.UnitPriceMinor, &l.Currency, &l.DiscountPct, &l.TaxRate, &l.TaxCategory, &l.BillingModel,
		&l.IntervalMonths, &l.IntervalCount, &l.ComponentKind, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func readDealLineRows(ctx context.Context, tx pgx.Tx, dealID ids.DealID) ([]dealLineRow, error) {
	rows, err := tx.Query(ctx, `SELECT `+dealLineColumns+` FROM deal_line_item WHERE deal_id = $1 ORDER BY position, id`, dealID)
	if err != nil {
		return nil, fmt.Errorf("read deal lines: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dealLineRow, error) { return scanDealLine(row) })
	if err != nil {
		return nil, fmt.Errorf("scan deal lines: %w", err)
	}
	return out, nil
}

// dealLineFigures derives the lines' figures. A deal names no legal entity,
// so a category line reads today's rate for the default issuer. A rate the
// sheet lacks leaves the tax unknown.
func (s *Store) dealLineFigures(ctx context.Context, tx pgx.Tx, lines []dealLineRow) ([]DealLineInput, DealCommercialFigures, error) {
	basis := s.draftTaxBasis(ctx, tx, nil)
	inputs := make([]DealLineInput, 0, len(lines))
	for _, line := range lines {
		in := DealLineInput{
			Quantity: line.Quantity, UnitPriceMinor: line.UnitPriceMinor, DiscountPct: line.DiscountPct,
			TaxRate: line.TaxRate, BillingModel: line.BillingModel, IntervalMonths: line.IntervalMonths,
			IntervalCount: line.IntervalCount,
		}
		if line.TaxCategory != nil {
			rate, err := sheetTaxRate(ctx, tx, basis, *line.TaxCategory)
			var missing *TaxRateMissingError
			switch {
			case errors.As(err, &missing):
				in.TaxRate = nil
			case err != nil:
				return nil, DealCommercialFigures{}, err
			default:
				in.TaxRate = &rate
			}
		}
		inputs = append(inputs, in)
	}
	figures, err := DealCommercialTotals(inputs)
	return inputs, figures, err
}

// GetDealLines reads a deal's lines and the value they add up to.
func (s *Store) GetDealLines(ctx context.Context, dealID ids.DealID) (crmcontracts.DealCommercial, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	var out crmcontracts.DealCommercial
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		var err error
		out, err = s.dealCommercialForCaller(ctx, tx, dealID)
		return err
	})
	return out, err
}

// dealAmountReadable answers whether the caller may read this deal's amount.
// Every price and figure of its lines reconstructs that amount, so the same
// mask withholds them.
func dealAmountReadable(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (bool, error) {
	args := []any{dealID}
	clause, masked, err := auth.MaskExcludedClause(ctx, "deal", "amount_minor", "deal", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil || !masked {
		return !masked, err
	}
	var readable bool
	if err := tx.QueryRow(ctx, `SELECT `+clause+` FROM deal WHERE deal.id = $1`, args...).Scan(&readable); err != nil {
		return false, fmt.Errorf("read the deal's amount mask: %w", err)
	}
	return readable, nil
}

// dealCommercialForCaller is the commercial state as this caller may read it.
func (s *Store) dealCommercialForCaller(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (crmcontracts.DealCommercial, error) {
	head, err := readDealCommercialHead(ctx, tx, dealID)
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	lines, err := readDealLineRows(ctx, tx, dealID)
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	inputs, figures, err := s.dealLineFigures(ctx, tx, lines)
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	readable, err := dealAmountReadable(ctx, tx, dealID)
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	writable, err := auth.WritableSubset(ctx, tx, dealTable, []ids.UUID{dealID.UUID})
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	basis, err := dealAmountBasis(ctx, tx, dealID, head, readable)
	if err != nil {
		return crmcontracts.DealCommercial{}, err
	}
	out := crmcontracts.DealCommercial{
		DealId:             openapi_types.UUID(dealID.UUID),
		Mode:               crmcontracts.DealCommercialMode(head.Mode),
		AmountBasis:        basis,
		CommercialRevision: head.Revision,
		Currency:           commercialCurrency(head, lines),
		Lines:              make([]crmcontracts.DealLine, 0, len(lines)),
		Complete:           len(figures.Missing) == 0,
		Missing:            make([]crmcontracts.DealCommercialMissing, 0, len(figures.Missing)),
		MoneyWithheld:      !readable,
		Writable:           writable[dealID.UUID] && head.Status == string(DealOpen) && readable,
	}
	for _, reason := range figures.Missing {
		out.Missing = append(out.Missing, crmcontracts.DealCommercialMissing(reason))
	}
	for i, line := range lines {
		line.TaxRate = inputs[i].TaxRate
		wire, err := dealLineWire(line, figures.Lines[i], readable)
		if err != nil {
			return crmcontracts.DealCommercial{}, err
		}
		out.Lines = append(out.Lines, wire)
	}
	if readable && head.Mode == modeLines {
		out.CommittedNetMinor, out.ArrMinor = figures.CommittedNetMinor, figures.ArrMinor
		out.TaxMinor, out.GrossMinor = figures.TaxMinor, figures.GrossMinor
		out.KnownCommittedNetMinor = &figures.KnownCommittedNetMinor
	}
	return out, nil
}

// commercialCurrency is the currency the lines are priced in, else the deal's.
func commercialCurrency(head dealCommercialHead, lines []dealLineRow) *string {
	if len(lines) > 0 {
		return &lines[0].Currency
	}
	return head.Currency
}

// dealAmountBasis says what the deal's amount means. A legacy amount equal to
// an accepted offer's gross came from that offer; any other legacy amount was
// typed by somebody whose basis nobody recorded. A reader the amount is
// masked from learns only what the mode says: comparing the amount to an
// offer's gross would tell them the amount.
func dealAmountBasis(ctx context.Context, tx pgx.Tx, dealID ids.DealID, head dealCommercialHead, readable bool) (*crmcontracts.DealCommercialAmountBasis, error) {
	basis := func(b crmcontracts.DealCommercialAmountBasis) (*crmcontracts.DealCommercialAmountBasis, error) {
		return &b, nil
	}
	switch {
	case head.Mode == modeLines:
		return basis(crmcontracts.DealCommercialAmountBasisLinesNetCommitted)
	case !readable || head.AmountMinor == nil:
		return nil, nil
	case head.Mode == modeEstimate:
		return basis(crmcontracts.DealCommercialAmountBasisEstimate)
	}
	var fromOffer bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM offer WHERE deal_id = $1 AND status = 'accepted' AND archived_at IS NULL
		                 AND gross_minor = $2 AND currency = $3)`,
		dealID, *head.AmountMinor, head.Currency).Scan(&fromOffer); err != nil {
		return nil, fmt.Errorf("read the deal's amount basis: %w", err)
	}
	if fromOffer {
		return basis(crmcontracts.DealCommercialAmountBasisOfferGross)
	}
	return basis(crmcontracts.DealCommercialAmountBasisUnknown)
}

// dealLineWire renders one line, with the tax rate it is figured at. A reader
// the amount is masked from gets the words and whether the price repeats. They
// get no number, because a quantity beside a catalogue price rebuilds the amount.
func dealLineWire(line dealLineRow, figures DealLineFigures, readable bool) (crmcontracts.DealLine, error) {
	out := crmcontracts.DealLine{
		Id: openapi_types.UUID(line.ID), Position: line.Position, ProductId: uuidPtr(line.ProductID),
		CatalogName: line.CatalogName, Sku: line.SKU, Description: line.Description, Unit: line.Unit,
		Currency: line.Currency, Version: &line.Version, CreatedAt: line.CreatedAt, UpdatedAt: line.UpdatedAt,
		Missing: make([]crmcontracts.DealLineMissing, 0, len(figures.Missing)),
	}
	for _, reason := range figures.Missing {
		out.Missing = append(out.Missing, crmcontracts.DealLineMissing(reason))
	}
	if line.BillingModel != nil {
		out.BillingModel = new(crmcontracts.DealLineBillingModel(*line.BillingModel))
	}
	if line.TaxCategory != nil {
		out.TaxCategory = new(crmcontracts.DealLineTaxCategory(*line.TaxCategory))
	}
	if line.ComponentKind != nil {
		out.ComponentKind = new(crmcontracts.DealLineComponentKind(*line.ComponentKind))
	}
	if !readable {
		return out, nil
	}
	return withLineNumbers(out, line, figures)
}

// withLineNumbers adds every number of a line, for a reader who may see them.
func withLineNumbers(out crmcontracts.DealLine, line dealLineRow, figures DealLineFigures) (crmcontracts.DealLine, error) {
	var err error
	if line.Quantity != nil {
		if out.Quantity, err = wireFloat("quantity", *line.Quantity); err != nil {
			return crmcontracts.DealLine{}, err
		}
	}
	if line.IntervalMonths != nil {
		out.BillingIntervalMonths = new(crmcontracts.DealLineBillingIntervalMonths(*line.IntervalMonths))
	}
	out.IntervalCount = line.IntervalCount
	out.UnitPriceMinor = line.UnitPriceMinor
	out.LineNetMinor, out.CommittedNetMinor, out.ArrMinor = figures.NetMinor, figures.CommittedNetMinor, figures.ArrMinor
	if out.DiscountPct, err = wireFloat("discount_pct", line.DiscountPct); err != nil {
		return crmcontracts.DealLine{}, err
	}
	if line.TaxRate != nil {
		if out.TaxRate, err = wireFloat("tax_rate", *line.TaxRate); err != nil {
			return crmcontracts.DealLine{}, err
		}
	}
	return out, nil
}

func wireFloat(field, value string) (*float64, error) {
	v, err := parseWireDecimal(field, value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
