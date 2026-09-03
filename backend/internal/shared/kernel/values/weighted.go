// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"errors"
	"fmt"
	"math/big"
)

// WeightedValue is formulas §6: baseMinor × winProbability/100, rounded half
// away from zero.
//
// It lives in the shared tier because three surfaces need the same figure —
// the account roll-up, the report engine, and a forecast snapshot — and they
// sit in different modules, which never import one another. A money formula
// with one implementation per caller is one answer per caller, and they
// diverge on rounding first: a one-minor-unit disagreement between two screens
// showing the same pipeline, which nobody can reproduce on demand.
//
// The report engine's SQL spelling cannot be folded in, because an aggregate
// has to compute in the database. That one is a declared mirror, held in both
// directions by TestTheTwoSpellingsOfWeightedValueAgree
// (compose/weightedvalueparity_integration_test.go). Every Go caller shares
// THIS one, so there are two spellings and not four.
//
// EXACT big.Int arithmetic, never a native int64 multiply. amount_minor is
// contract-unbounded, so baseMinor × winProbability can exceed int64 before
// the division would have widened it back, and a silent wraparound there puts
// a wrong number in a money total.
func WeightedValue(baseMinor int64, winProbability int) (int64, error) {
	product := new(big.Int).Mul(big.NewInt(baseMinor), big.NewInt(int64(winProbability)))
	rounded := divRoundHalfAwayFromZero(product, big.NewInt(100))
	if !rounded.IsInt64() {
		return 0, fmt.Errorf("%w: a %d-minor-unit amount at %d%%; correct the deal amount before retrying",
			ErrWeightedValueOutOfRange, baseMinor, winProbability)
	}
	return rounded.Int64(), nil
}

// ErrWeightedValueOutOfRange marks the ARITHMETIC's own refusal, as distinct
// from a caller rejecting an input before the multiply ever runs. A test that
// accepted any error here would keep passing with the overflow check deleted,
// as long as something else refused first.
var ErrWeightedValueOutOfRange = errors.New("weighted pipeline value exceeds the representable money range")

// divRoundHalfAwayFromZero is numerator/denominator with the quotient rounded
// half away from zero. The denominator is always a positive power of ten.
//
// Half AWAY FROM ZERO rather than Go's default truncation or Postgres's
// round(): the direction has to match the SQL mirror above, and truncation
// would bias every weighted total downward by up to one minor unit per deal.
func divRoundHalfAwayFromZero(numerator, denominator *big.Int) *big.Int {
	negative := numerator.Sign() < 0
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	remainder.Abs(remainder)
	remainder.Lsh(remainder, 1) // 2·|remainder| ≥ denominator ⇔ the dropped fraction is ≥ half
	if remainder.Cmp(denominator) < 0 {
		return quotient
	}
	if negative {
		return quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient.Add(quotient, big.NewInt(1))
}
