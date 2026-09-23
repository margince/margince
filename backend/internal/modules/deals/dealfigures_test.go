// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A card states the figures the record page states, and withholds what the
// record page withholds. A currency is a mask's subject in its own right —
// masking it alone drags nothing along — so a card that only ever answered the
// amount's mask printed the units the record page declines to give.
//
// No transaction: the seat holds no update verb, so there is no row whose write
// authority could lift a condition and nothing to ask the database.
func TestACardWithholdsTheCurrencyAMaskNamesOnItsOwn(t *testing.T) {
	t.Parallel()
	deal, amount := ids.NewV7(), int64(250000)
	figures := map[ids.UUID]DealFigures{deal: {AmountMinor: &amount, Currency: "EUR"}}
	ctx := principal.WithActor(context.Background(), dealSeatMasking(dealCurrencyField))

	if err := maskFigures(ctx, nil, figures); err != nil {
		t.Fatalf("masking a page of cards: %v", err)
	}
	if got := figures[deal].Currency; got != "" {
		t.Errorf("currency = %q, want it withheld", got)
	}
	if figures[deal].AmountMinor == nil {
		t.Error("a currency mask took the amount with it; the group is directed, not symmetric")
	}
}

// Every field the CARD carries and the deal can withhold is one this read
// nulls. The card names no masked_fields, so a figure it fails to withhold is
// not merely unnamed — it is the value itself, on a surface that lists deals in
// bulk.
func TestEveryWithheldFieldTheCardCarriesIsNulled(t *testing.T) {
	t.Parallel()
	carried := 0
	for field := range dealWithholds {
		name, carries := figuresFieldNamed(field)
		if !carries {
			continue
		}
		carried++
		if _, nulled := dealFigureWithholds[field]; !nulled {
			t.Errorf("a card carries %s as %s and this read cannot withhold it: a mask on the field "+
				"prints on the Worklist the figure the record page refuses", field, name)
		}
	}
	if carried == 0 {
		t.Fatal("no withheld deal field was recognised on DealFigures, so this census read an empty " +
			"subject and would agree with a read that withholds nothing")
	}
}

// figuresFieldNamed answers which field of a card a wire name is the copy of.
// It compares the two spellings one wire field can have in Go — CompanyId and
// CompanyID both answer to company_id — because a census that read only one
// would report a column the card carries as one it does not, and pass.
func figuresFieldNamed(field string) (string, bool) {
	spelt := strings.ReplaceAll(field, "_", "")
	for _, carried := range reflect.VisibleFields(reflect.TypeFor[DealFigures]()) {
		if strings.EqualFold(carried.Name, spelt) {
			return carried.Name, true
		}
	}
	return "", false
}
