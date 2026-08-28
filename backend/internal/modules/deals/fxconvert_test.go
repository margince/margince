// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The conversion's arithmetic, proven without a database.
//
// These cases moved here with the function they test. Two surfaces used to
// convert money to the base currency their own way, and the first divergence
// anyone predicted was rounding — so the rounding rule is now proven once,
// beside the one implementation, rather than in whichever caller happened to
// have a test.

import (
	"math"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// numericRate builds a pgtype.Numeric the way pgx materializes a stored
// numeric: coefficient × 10^exp.
func numericRate(coefficient int64, exp int32) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(coefficient), Exp: exp, Valid: true}
}

func TestConvertToBase(t *testing.T) {
	cases := []struct {
		name        string
		amountMinor int64
		rate        pgtype.Numeric
		want        int64
	}{
		{name: "rate of 1.0 is a passthrough", amountMinor: 123456, rate: numericRate(1, 0), want: 123456},
		{name: "positive half rounds away from zero", amountMinor: 1, rate: numericRate(5, -1), want: 1},
		{name: "negative half rounds away from zero", amountMinor: -1, rate: numericRate(5, -1), want: -1},
		{name: "positive one-and-half rounds up", amountMinor: 3, rate: numericRate(5, -1), want: 2},
		{name: "negative one-and-half rounds down", amountMinor: -3, rate: numericRate(5, -1), want: -2},
		{name: "zero amount converts to zero at any rate", amountMinor: 0, rate: numericRate(137, -2), want: 0},
		{name: "positive-exponent coefficient scales up", amountMinor: 3, rate: numericRate(2, 3), want: 6000},
		{
			// Above float64's 2^53 exact-integer ceiling the old float
			// conversion would silently drop the odd minor unit; the exact
			// decimal path must keep it.
			name:        "amount past 2^53 keeps its last minor unit",
			amountMinor: 9_007_199_254_740_993, // 2^53 + 1
			rate:        numericRate(1, 0),
			want:        9_007_199_254_740_993,
		},
		{
			// Full numeric(20,10) scale: 0.0000000001 × 15,000,000,000
			// lands exactly on the 1.5 tie, which rounds away from zero
			// only when judged on exact decimal digits.
			name:        "ten-decimal rate rounds on exact digits",
			amountMinor: 15_000_000_000,
			rate:        numericRate(1, -10),
			want:        2, // 1.5 exactly, away from zero
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertToBase(tc.amountMinor, tc.rate)
			if err != nil {
				t.Fatalf("ConvertToBase(%d, %v): %v", tc.amountMinor, tc.rate, err)
			}
			if got != tc.want {
				t.Errorf("ConvertToBase(%d, %v) = %d, want %d", tc.amountMinor, tc.rate, got, tc.want)
			}
		})
	}
}

func TestConvertToBaseRefusesDishonestResults(t *testing.T) {
	t.Run("non-finite rate is refused", func(t *testing.T) {
		if _, err := ConvertToBase(100, pgtype.Numeric{NaN: true, Valid: true}); err == nil {
			t.Error("NaN rate converted — must refuse, a money total can never absorb it")
		}
	})
	t.Run("overflowing conversion is refused", func(t *testing.T) {
		// max-int64 minor units at rate 100 cannot fit int64; a wrapped
		// (silently truncated) figure would be a lie about money.
		if _, err := ConvertToBase(math.MaxInt64, numericRate(1, 2)); err == nil {
			t.Error("overflowing conversion returned a value — must refuse")
		}
	})
}
