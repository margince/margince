// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal's product lines are worth. The per-line arithmetic is the
// offer engine's own (LineTotals), so a deal and the offer made from it round
// the same way. What this adds is the answer for a line that is not complete
// yet: an unknown input makes the figure it feeds unknown, never zero.

import (
	"math/big"
	"slices"
)

// The reasons a deal's value is not complete yet, as the contract names them.
const (
	missingLines         = "lines"
	missingQuantity      = "quantity"
	missingUnitPrice     = "unit_price"
	missingBillingModel  = "billing_model"
	missingIntervalCount = "interval_count"
	missingTaxRate       = "tax_rate"
)

// DealLineInput is one deal line as the figures engine reads it. A nil
// quantity, price or tax rate is a value nobody knows yet.
type DealLineInput struct {
	Quantity       *string
	UnitPriceMinor *int64
	DiscountPct    string
	TaxRate        *string
	BillingModel   *string
	IntervalMonths *int
	IntervalCount  *int
}

// DealLineFigures are one line's figures. NetMinor is per billing period for
// a recurring line. Each figure is nil when an input it needs is unknown.
type DealLineFigures struct {
	NetMinor          *int64
	CommittedNetMinor *int64
	ArrMinor          *int64
	CommittedTaxMinor *int64
	Missing           []string
}

// DealCommercialFigures are the sums over a deal's lines. CommittedNetMinor
// is the deal's value, nil until every line is complete; the known part is
// KnownCommittedNetMinor either way.
type DealCommercialFigures struct {
	CommittedNetMinor      *int64
	KnownCommittedNetMinor int64
	ArrMinor               *int64
	TaxMinor               *int64
	GrossMinor             *int64
	Missing                []string
	Lines                  []DealLineFigures
}

// DealLineTotals derives one line's figures.
func DealLineTotals(line DealLineInput) (DealLineFigures, error) {
	var out DealLineFigures
	if line.Quantity == nil {
		out.Missing = append(out.Missing, missingQuantity)
	}
	if line.UnitPriceMinor == nil {
		out.Missing = append(out.Missing, missingUnitPrice)
	}
	if line.BillingModel == nil {
		out.Missing = append(out.Missing, missingBillingModel)
	}
	if line.Quantity == nil || line.UnitPriceMinor == nil {
		return out, nil
	}
	rate := zeroPct
	if line.TaxRate != nil {
		rate = *line.TaxRate
	}
	figures, err := LineTotals(OfferLineInput{
		Quantity: *line.Quantity, UnitPriceMinor: *line.UnitPriceMinor, DiscountPct: line.DiscountPct, TaxRate: rate,
	})
	if err != nil {
		return DealLineFigures{}, err
	}
	out.NetMinor = &figures.NetMinor
	periods, arr, missing, err := linePeriods(line, figures.NetMinor)
	if err != nil {
		return DealLineFigures{}, err
	}
	out.ArrMinor = arr
	out.Missing = append(out.Missing, missing...)
	if periods == nil {
		return out, nil
	}
	if out.CommittedNetMinor, err = timesPeriods(figures.NetMinor, *periods, "committed_net_minor"); err != nil {
		return DealLineFigures{}, err
	}
	if line.TaxRate == nil {
		out.Missing = append(out.Missing, missingTaxRate)
		return out, nil
	}
	if out.CommittedTaxMinor, err = timesPeriods(figures.TaxMinor, *periods, "committed_tax_minor"); err != nil {
		return DealLineFigures{}, err
	}
	return out, nil
}

// linePeriods answers how many times a line's net is paid, and its annual
// recurring value. Both are nil while the billing is not stated; a recurring
// line with no term has an annual value and no committed count.
func linePeriods(line DealLineInput, netMinor int64) (periods *int64, arr *int64, missing []string, err error) {
	if line.BillingModel == nil {
		return nil, nil, nil, nil
	}
	var zeroArr int64
	switch *line.BillingModel {
	case BillingOneTime:
		one := int64(1)
		return &one, &zeroArr, nil, nil
	case BillingRecurring:
		if line.IntervalMonths == nil || *line.IntervalMonths <= 0 {
			return nil, nil, []string{missingBillingModel}, nil
		}
		annual, err := timesPeriods(netMinor, int64(monthsPerYear / *line.IntervalMonths), "arr_minor")
		if err != nil {
			return nil, nil, nil, err
		}
		if line.IntervalCount == nil {
			return nil, annual, []string{missingIntervalCount}, nil
		}
		count := int64(*line.IntervalCount)
		return &count, annual, nil, nil
	}
	return nil, nil, []string{missingBillingModel}, nil
}

func timesPeriods(minor, periods int64, figure string) (*int64, error) {
	product := new(big.Int).Mul(big.NewInt(minor), big.NewInt(periods))
	v, err := minorFromBig(product, figure, "quantity", "unit_price_minor", intervalCountField)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// DealCommercialTotals sums a deal's lines. A deal with no line has no value.
func DealCommercialTotals(lines []DealLineInput) (DealCommercialFigures, error) {
	out := DealCommercialFigures{Lines: make([]DealLineFigures, 0, len(lines))}
	committed, known, arr, tax := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	committedComplete, arrComplete, taxComplete := len(lines) > 0, len(lines) > 0, len(lines) > 0
	if len(lines) == 0 {
		out.Missing = []string{missingLines}
	}
	for _, line := range lines {
		figures, err := DealLineTotals(line)
		if err != nil {
			return DealCommercialFigures{}, err
		}
		out.Lines = append(out.Lines, figures)
		for _, reason := range figures.Missing {
			if !slices.Contains(out.Missing, reason) {
				out.Missing = append(out.Missing, reason)
			}
		}
		committedComplete = addKnown(committed, figures.CommittedNetMinor, committedComplete)
		if figures.CommittedNetMinor != nil {
			known.Add(known, big.NewInt(*figures.CommittedNetMinor))
		}
		arrComplete = addKnown(arr, figures.ArrMinor, arrComplete)
		taxComplete = addKnown(tax, figures.CommittedTaxMinor, taxComplete)
	}
	var err error
	if out.KnownCommittedNetMinor, err = minorFromBig(known, "known_committed_net_minor", "line_items"); err != nil {
		return DealCommercialFigures{}, err
	}
	if err = setCompleteSum(&out.CommittedNetMinor, committed, committedComplete, "committed_net_minor"); err != nil {
		return DealCommercialFigures{}, err
	}
	if err = setCompleteSum(&out.ArrMinor, arr, arrComplete, "arr_minor"); err != nil {
		return DealCommercialFigures{}, err
	}
	if !committedComplete || !taxComplete {
		return out, nil
	}
	if err = setCompleteSum(&out.TaxMinor, tax, true, "tax_minor"); err != nil {
		return DealCommercialFigures{}, err
	}
	gross := new(big.Int).Add(committed, tax)
	err = setCompleteSum(&out.GrossMinor, gross, true, "gross_minor")
	return out, err
}

// addKnown adds a figure to its sum and answers whether the sum is still
// complete.
func addKnown(sum *big.Int, figure *int64, complete bool) bool {
	if figure == nil {
		return false
	}
	sum.Add(sum, big.NewInt(*figure))
	return complete
}

// setCompleteSum stores a sum every line stated. A figure a line left unknown
// stays nil, because a partial sum is a number nobody stated.
func setCompleteSum(figure **int64, sum *big.Int, complete bool, name string) error {
	if !complete {
		return nil
	}
	v, err := minorFromBig(sum, name, lineItemsField)
	if err != nil {
		return err
	}
	*figure = &v
	return nil
}
