// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The reasons that say what is wrong inside one cell, so the hint fits the
// fault. They name an amount's decimal places or missing currency, and text
// that is no phone number. withCellReasons joins them to the catalog.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for a cell whose form is the fault.
const (
	ReasonAmountTooManyDecimals ReasonCode = "amount_too_many_decimals"
	ReasonAmountNeedsCurrency   ReasonCode = "amount_needs_currency"
	ReasonNotAPhone             ReasonCode = "not_a_phone"
)

// Parameter names the amount reasons share. The example is an amount written
// the way the row's currency takes it, built by the caller from that currency.
const (
	paramCurrency = "currency"
	paramExample  = "example"
)

// withCellReasons adds the cell reasons to the catalog and answers it.
func withCellReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, cellReasons)
	return catalog
}

var cellReasons = map[ReasonCode]reasonEntry{
	ReasonAmountTooManyDecimals: {params: []string{paramValue, paramCurrency, paramExample}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” has more decimal places than {currency} takes. Write it like {example}.",
		textlang.German:     "{value:q} hat mehr Nachkommastellen, als {currency} zulässt. Schreiben Sie den Betrag so: {example}",
		textlang.Vietnamese: "{value:q} có nhiều chữ số thập phân hơn mức {currency} cho phép. Hãy viết như {example}.",
	}},
	ReasonAmountNeedsCurrency: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "The amount “{value}” has no currency. Fill the currency column of this row with a code, like EUR.",
		textlang.German:     "Der Betrag {value:q} hat keine Währung. Füllen Sie die Währungsspalte dieser Zeile mit einem Code, etwa EUR.",
		textlang.Vietnamese: "Số tiền {value:q} chưa có loại tiền. Hãy điền mã vào cột tiền tệ của hàng này, ví dụ EUR.",
	}},
	ReasonNotAPhone: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a phone number. Write it with its country code, like +49 30 1234567.",
		textlang.German:     "{value:q} ist keine Telefonnummer. Schreiben Sie sie mit Landesvorwahl, zum Beispiel +49 30 1234567.",
		textlang.Vietnamese: "{value:q} không phải số điện thoại. Hãy viết kèm mã quốc gia, ví dụ +49 30 1234567.",
	}},
}
