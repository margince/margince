// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// citedPassageID is a passage id as numberPassages mints it, wherever the
// evidence puts it: bare, bracketed, or beside a quote.
var citedPassageID = regexp.MustCompile(`\bs\d+\b`)

// statedNumber is a decimal as a page prints one, currency symbol and trailing
// zeros aside: "$5.00" states 5, "1.0850" states 1.085. A comma may be either
// a decimal comma or a thousands separator, so printsNumber reads it both ways.
var statedNumber = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// ratePassages is a page's passages keyed by the id numberPassages gives each,
// read back off its output so the two can never number a page differently.
func ratePassages(pageText string) map[string]string {
	passages := map[string]string{}
	for _, line := range strings.Split(numberPassages(pageText), "\n") {
		id, text, found := strings.Cut(strings.TrimPrefix(line, "["), "] ")
		if found {
			passages[id] = text
		}
	}
	return passages
}

// misattributed names a row the gate admitted for subject whose evidence cites
// no passage stating every one of values. The gate asks only that a row cite
// something, so a right value cited at the wrong line passes it; this holds the
// value to the passage it was read from. A zero is not held, since a page states
// "not available" rather than a 0. Empty when the citation holds.
func misattributed(passages map[string]string, subject, evidence string, values ...string) string {
	for _, id := range citedPassageID.FindAllString(evidence, -1) {
		if statesEvery(passages[id], values) {
			return ""
		}
	}
	return fmt.Sprintf("%q cites %s, which does not state it", subject, strings.TrimSpace(evidence))
}

// statesEvery says passage prints each non-zero value among its numbers.
func statesEvery(passage string, values []string) bool {
	stated := statedNumber.FindAllString(passage, -1)
	for _, value := range values {
		want, ok := plainRat(strings.TrimSpace(value))
		if ok && want.Sign() == 0 {
			continue
		}
		if !ok || !printsNumber(stated, want) {
			return false
		}
	}
	return true
}

func printsNumber(stated []string, want *big.Rat) bool {
	for _, s := range stated {
		for _, reading := range []string{strings.ReplaceAll(s, ",", ""), strings.ReplaceAll(s, ",", ".")} {
			if got, ok := plainRat(reading); ok && got.Cmp(want) == 0 {
				return true
			}
		}
	}
	return false
}

// plainRat reads a plain decimal no longer than a stated rate may be, so an
// exponent a page or a reply prints is never expanded by its digits.
func plainRat(s string) (*big.Rat, bool) {
	if !values.PlainDecimal(s, fxRateIntDigits, fxStatedFracDigits) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}
