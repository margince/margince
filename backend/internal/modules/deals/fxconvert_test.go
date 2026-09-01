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

// TestConvertToBaseAppliesBothMinorUnitScales is the case a same-scale table
// cannot reach, and the one a mixed pipeline meets first.
//
// A stored rate says what one MAJOR unit is worth ("1 JPY = 0.006 EUR" is how
// the ingestion prompt reads a rates page), while both amounts count MINOR
// units. JPY has none and EUR has two, so a bare minor × rate answers a
// hundredth of the truth — and it answers it in the direction that makes a yen
// deal look small on a page that ranks by money.
func TestConvertToBaseAppliesBothMinorUnitScales(t *testing.T) {
	// ¥5,000,000 is five million yen. At 1 JPY = 0.006 EUR that is €30,000,
	// which in EUR minor units is 3,000,000.
	got, err := ConvertToBase(5_000_000, numericRate(6, -3), "JPY", "EUR")
	if err != nil {
		t.Fatalf("converting yen to euro: %v", err)
	}
	if got != 3_000_000 {
		t.Errorf("¥5,000,000 = %d EUR minor units, want 3000000 (€30,000); "+
			"30000 would be €300, the scale error a bare minor × rate makes", got)
	}
	// And back the other way, so the fix cannot be a one-directional fudge:
	// €30,000 at 1 EUR = 166.6667 JPY is about ¥5,000,001 — whole yen, because
	// JPY has no minor unit to carry a fraction.
	back, err := ConvertToBase(3_000_000, numericRate(1_666_667, -4), "EUR", "JPY")
	if err != nil {
		t.Fatalf("converting euro to yen: %v", err)
	}
	if back != 5_000_001 {
		t.Errorf("€30,000 = %d JPY minor units, want 5000001 whole yen", back)
	}
}

func TestConvertToBase(t *testing.T) {
	cases := []struct {
		name        string
		amountMinor int64
		rate        pgtype.Numeric
		// from and to are the currencies the amounts count minor units of.
		// Empty means EUR on both sides — the same-scale case these rounding
		// and overflow cases are about, stated once here rather than repeated
		// on every row where it is not the subject.
		from, to string
		want     int64
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
			from, to := tc.from, tc.to
			if from == "" {
				from, to = "EUR", "EUR"
			}
			got, err := ConvertToBase(tc.amountMinor, tc.rate, from, to)
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
		if _, err := ConvertToBase(100, pgtype.Numeric{NaN: true, Valid: true}, "EUR", "EUR"); err == nil {
			t.Error("NaN rate converted — must refuse, a money total can never absorb it")
		}
	})
	t.Run("overflowing conversion is refused", func(t *testing.T) {
		// max-int64 minor units at rate 100 cannot fit int64; a wrapped
		// (silently truncated) figure would be a lie about money.
		if _, err := ConvertToBase(math.MaxInt64, numericRate(1, 2), "EUR", "EUR"); err == nil {
			t.Error("overflowing conversion returned a value — must refuse")
		}
	})
}

// TestConvertToBaseRefusesRatherThanWrapping is the property both callers rest
// their own policy on.
//
// They differ in what they DO with the refusal — one fails the whole read, the
// other leaves that deal unpriced and counted — and neither is safe if the
// engine can return a wrapped number instead of refusing. So the refusal is
// proven here, once, rather than assumed at two call sites.
func TestConvertToBaseRefusesRatherThanWrapping(t *testing.T) {
	// The largest rate the column holds, against an amount near the top of its
	// own range: a product no int64 can carry.
	if _, err := ConvertToBase(math.MaxInt64, numericRate(9_999_999_999, 0), "EUR", "EUR"); err == nil {
		t.Error("a product past int64 returned a value — a wrapped figure is a plausible-looking wrong " +
			"number, which is worse than no number")
	}
	// And the boundary the other way: a product that exactly fits is answered.
	if got, err := ConvertToBase(math.MaxInt64, numericRate(1, 0), "EUR", "EUR"); err != nil || got != math.MaxInt64 {
		t.Errorf("the largest representable product = %d, %v; want it answered, not refused", got, err)
	}
}
