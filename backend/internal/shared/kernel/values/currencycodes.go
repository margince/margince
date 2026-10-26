// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"maps"
	"slices"
)

// assignedCurrencies are the ISO-4217 alphabetic codes a record may be priced in.
// BGN, HRK, CUC, SLL and ZWL are withdrawn, but stored records carry them.
// The browser picker offers them too, and it may only offer codes the server accepts.
var assignedCurrencies = func() map[string]struct{} {
	codes := map[string]struct{}{}
	for _, code := range []string{
		"AED", "AFN", "ALL", "AMD", "ANG", "AOA", "ARS", "AUD", "AWG", "AZN",
		"BAM", "BBD", "BDT", "BGN", "BHD", "BIF", "BMD", "BND", "BOB", "BOV",
		"BRL", "BSD", "BTN", "BWP", "BYN", "BZD", "CAD", "CDF", "CHE", "CHF",
		"CHW", "CLF", "CLP", "CNY", "COP", "COU", "CRC", "CUC", "CUP", "CVE", "CZK",
		"DJF", "DKK", "DOP", "DZD", "EGP", "ERN", "ETB", "EUR", "FJD", "FKP",
		"GBP", "GEL", "GHS", "GIP", "GMD", "GNF", "GTQ", "GYD", "HKD", "HNL",
		"HRK", "HTG", "HUF", "IDR", "ILS", "INR", "IQD", "IRR", "ISK", "JMD",
		"JOD", "JPY", "KES", "KGS", "KHR", "KMF", "KPW", "KRW", "KWD", "KYD",
		"KZT", "LAK", "LBP", "LKR", "LRD", "LSL", "LYD", "MAD", "MDL", "MGA",
		"MKD", "MMK", "MNT", "MOP", "MRU", "MUR", "MVR", "MWK", "MXN", "MXV",
		"MYR", "MZN", "NAD", "NGN", "NIO", "NOK", "NPR", "NZD", "OMR", "PAB",
		"PEN", "PGK", "PHP", "PKR", "PLN", "PYG", "QAR", "RON", "RSD", "RUB",
		"RWF", "SAR", "SBD", "SCR", "SDG", "SEK", "SGD", "SHP", "SLE", "SOS",
		"SLL", "SRD", "SSP", "STN", "SVC", "SYP", "SZL", "THB", "TJS", "TMT", "TND",
		"TOP", "TRY", "TTD", "TWD", "TZS", "UAH", "UGX", "USD", "USN", "UYI",
		"UYU", "UYW", "UZS", "VED", "VES", "VND", "VUV", "WST", "XAF", "XAG",
		"XAU", "XBA", "XBB", "XBC", "XBD", "XCD", "XCG", "XDR", "XOF", "XPD",
		"XPF", "XPT", "XSU", "XTS", "XUA", "XXX", "YER", "ZAR", "ZMW", "ZWG", "ZWL",
	} {
		codes[code] = struct{}{}
	}
	return codes
}()

// AssignableCurrency answers whether s may be newly set or changed to: the
// shape and ISO assigns the code. A stored value outside the list stays valid
// for reads and for edits that leave the currency alone.
func AssignableCurrency(s string) bool {
	_, listed := assignedCurrencies[s]
	return listed && ValidCurrency(s)
}

// RequireAssignableCurrency is AssignableCurrency as the ParseError a writer
// returns for a currency it is setting.
func RequireAssignableCurrency(s string) error {
	if AssignableCurrency(s) {
		return nil
	}
	return &ParseError{
		Field:   currencyField,
		Code:    "currency_unassigned",
		Message: "currency must be an assigned ISO-4217 code, such as EUR",
	}
}

// AssignedCurrencies lists the assignable codes in order, for the gate that
// holds the browser's mirror of them.
func AssignedCurrencies() []string {
	return slices.Sorted(maps.Keys(assignedCurrencies))
}
