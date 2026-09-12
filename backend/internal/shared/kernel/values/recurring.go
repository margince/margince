// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// The monthly reading of an annual recurring figure.
//
// ARR is stored annually because that is how it is negotiated and how it is
// reported, but a reader comparing a subscription against a monthly cost wants
// it monthly, so the product shows both. The division is the entire content of
// this file, and it lives here because it happens twice — once in Go for
// exports and reports, once in TypeScript for the record page — and the two
// have to produce the same number for the same input.
//
// The rounding is HALF UP on the minor unit, and there is no way to divide
// twelve into an arbitrary annual figure without one: EUR 100.00 a year is
// 8.333… a month, and the product has to print something. Half up is chosen
// because it is what a reader doing the division on paper does, and because
// the alternative (banker's rounding) would make two adjacent figures round in
// opposite directions for no reason a reader could see.
//
// The result is APPROXIMATE whenever the division was not exact, and that fact
// travels with it rather than being left for the caller to re-derive. A
// monthly figure printed as exact when twelve of it do not add back to the
// annual one is a figure somebody will eventually multiply and query.

// MonthlyEquivalent divides an annual recurring figure into a monthly one, in
// the same minor units. Approximate reports whether the division lost anything,
// so a surface can mark the figure rather than implying twelve of it reproduce
// the year.
//
// It works on minor units alone and never looks at the currency: the scale is
// already applied on both sides of the division, so dividing minor units by
// twelve is the same operation for a currency with no minor unit as for one
// with three.
//
// A negative input cannot occur — deal_expected_arr_nonnegative and its
// contract twin refuse one in the database, and moneyPairError refuses one
// before that — but the truncation below rounds toward zero, so the half-up
// correction is written to hold for a negative anyway rather than silently
// meaning half-down there.
func MonthlyEquivalent(annualMinor int64) (monthlyMinor int64, approximate bool) {
	const monthsPerYear = 12
	quotient := annualMinor / monthsPerYear
	remainder := annualMinor % monthsPerYear
	if remainder == 0 {
		return quotient, false
	}
	// Half up: a remainder of 6 or more (in absolute terms) carries. Doubling
	// the remainder rather than comparing against 6 keeps the test exact for
	// any divisor, and cannot overflow because the remainder is under 12.
	if remainder < 0 {
		if -remainder*2 >= monthsPerYear {
			quotient--
		}
		return quotient, true
	}
	if remainder*2 >= monthsPerYear {
		quotient++
	}
	return quotient, true
}
