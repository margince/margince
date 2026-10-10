// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The ranges of the numbers an offer line and a product carry, as the contract
// and the columns declare them. A figure outside one is refused by name, so it
// never reaches a CHECK that can only say "check the picklist".
const (
	// maxPriceMinor is the largest minor-unit amount a JSON number holds without loss.
	maxPriceMinor = values.MaxExactInteger
	// The columns' widths, numeric(14,3) and numeric(5,2), in thousandths and
	// hundredths.
	maxQuantityMilli = 99999999999999
	maxPercentCenti  = 99999
)

// FigureError maps to 422: a number outside the range or precision its field
// declares.
type FigureError struct{ Field, Rule string }

func (e *FigureError) Error() string { return e.Field + " " + e.Rule }

// FieldFault names the figure and the rule it broke.
func (e *FigureError) FieldFault() (field, code, message string) {
	return e.Field, "out_of_range", e.Error()
}

// figureLimit is one field's range. A nil bound is open on that side.
type figureLimit struct {
	field        string
	min, max     *big.Rat
	minExclusive bool
	places       int
}

var (
	zero          = big.NewRat(0, 1)
	quantityLimit = figureLimit{field: "quantity", min: zero, max: big.NewRat(maxQuantityMilli, 1000), minExclusive: true, places: 3}
	discountLimit = figureLimit{field: "discount_pct", min: zero, max: big.NewRat(100, 1), places: 2}
	taxLimit      = figureLimit{field: "tax_rate", min: zero, max: big.NewRat(maxPercentCenti, 100), places: 2}
	// The product's default carries the same bounds under its own name.
	defaultTaxLimit = figureLimit{field: "default_tax_rate", min: zero, max: big.NewRat(maxPercentCenti, 100), places: 2}
)

// wireDecimal renders a number as the caller wrote it. The column would round
// a longer one to its scale, so the figure is judged as sent, not as stored.
func wireDecimal(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// checkFigure refuses a decimal that is out of range or has more places than
// its column keeps.
func checkFigure(lim figureLimit, value string) error {
	rat, err := ratFromDecimal(lim.field, value)
	if err != nil {
		return err
	}
	if dot := strings.IndexByte(value, '.'); dot >= 0 && len(strings.TrimRight(value, "0"))-dot-1 > lim.places {
		return &FigureError{Field: lim.field, Rule: fmt.Sprintf("holds at most %d decimal places", lim.places)}
	}
	switch c := rat.Cmp(lim.min); {
	case c < 0 || (c == 0 && lim.minExclusive):
		return &FigureError{Field: lim.field, Rule: "must be " + belowRule(lim) + plain(lim.min, lim.places)}
	case rat.Cmp(lim.max) > 0:
		return &FigureError{Field: lim.field, Rule: "must be at most " + plain(lim.max, lim.places)}
	}
	return nil
}

// plain writes a bound without the zeros its column pads it with.
func plain(r *big.Rat, places int) string {
	s := r.FloatString(places)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func belowRule(lim figureLimit) string {
	if lim.minExclusive {
		return "greater than "
	}
	return "at least "
}

// checkPrice refuses a minor-unit amount outside 0 to the largest exact number.
func checkPrice(amountMinor int64) error {
	if amountMinor < 0 || amountMinor > maxPriceMinor {
		return &FigureError{Field: productPriceColumn, Rule: fmt.Sprintf("must be between 0 and %d", int64(maxPriceMinor))}
	}
	return nil
}

// checkCurrency refuses a currency that is not three upper-case letters.
func checkCurrency(code string) error {
	if !values.ValidCurrency(code) {
		return &FigureError{Field: currencyField, Rule: "must be a three-letter upper-case code, such as EUR"}
	}
	return nil
}

// checkLinePosition refuses a position before the first.
func checkLinePosition(position *int) error {
	if position != nil && *position < 1 {
		return &FigureError{Field: "position", Rule: "must be 1 or more"}
	}
	return nil
}

// checkLineFigures judges every number a line carries.
func checkLineFigures(line OfferLineInput) error {
	for _, c := range []struct {
		lim   figureLimit
		value string
	}{{quantityLimit, line.Quantity}, {discountLimit, line.DiscountPct}, {taxLimit, line.TaxRate}} {
		if err := checkFigure(c.lim, c.value); err != nil {
			return err
		}
	}
	return checkPrice(line.UnitPriceMinor)
}

// requireLineWords refuses a patch that blanks a line's description or unit,
// which creating the line refuses too, and stores the trimmed words.
func requireLineWords(in UpdateOfferLineInput) (UpdateOfferLineInput, error) {
	var err error
	if in.Description, err = trimmedWord("description", in.Description); err != nil {
		return in, err
	}
	in.Unit, err = trimmedWord("unit", in.Unit)
	return in, err
}

// trimmedWord answers a sent word trimmed, or refuses it when it is blank.
func trimmedWord(field string, word *string) (*string, error) {
	if word == nil {
		return word, nil
	}
	text, err := httperr.RequireNonBlank(field, *word)
	return &text, err
}
