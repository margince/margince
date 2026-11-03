// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package statedprice

import "testing"

// The deal in every case is worth EUR 48,000.00.
const dealMinor = 4_800_000

func TestATotalInTheDealsCurrencyThatIsNotTheDealValueIsFlagged(t *testing.T) {
	cases := []struct {
		words string
		want  int64
	}{
		{"We are happy to proceed with the Standard package at €42,000.", 4_200_000},
		{"Wir gehen mit dem Paket für 42.500,50 EUR weiter.", 4_250_050},
		{"Let's go ahead at EUR 45k for the whole rollout.", 4_500_000},
		{"Budget approved: 1.5 Mio. EUR.", 150_000_000},
		{"Final offer 39 999 euros.", 3_999_900},
	}
	for _, c := range cases {
		t.Run(c.words, func(t *testing.T) {
			got, ok := Differing(c.words, dealMinor, "EUR")
			if !ok || got != c.want {
				t.Errorf("Differing = %d, %v; want %d flagged", got, ok, c.want)
			}
		})
	}
}

func TestASumThatIsNotADifferentTotalIsNeverFlagged(t *testing.T) {
	cases := map[string]string{
		"the deal value":         "We confirm the order at €48,000.",
		"the deal value grouped": "Wir bestätigen 48.000 EUR.",
		"another currency":       "We can do $42,000 if that works.",
		"no currency":            "We will need 42,000 licences eventually.",
		"a deposit":              "We will pay a deposit of €10,000 next week.",
		"an instalment":          "The first instalment of €12,000 goes out in May.",
		"per month":              "We agreed to €4,000 per month.",
		"per seat":               "Fine with €40 per-seat pricing.",
		"monthly":                "€4,000 monthly works for us.",
		"German deposit":         "Die Anzahlung von 10.000 EUR kommt Freitag.",
		"Vietnamese instalment":  "Chúng tôi trả góp 12.000 EUR.",
		"two sums":               "Either €42,000 or €45,000, we will decide.",
		"a change to the price":  "We will pay €42 more than planned.",
		"a discount":             "We expect a discount of €2,000.",
		"a fee beside it":        "Travel costs of €1,500 are on us.",
		"a tax":                  "Plus €9,120 VAT, of course.",
		"German discount":        "Wir rechnen mit 2.000 EUR Rabatt.",
		"Vietnamese fee":         "Phí triển khai 3.000 EUR do chúng tôi trả.",
		"a word holding a mark":  "We expand to Europe 2027 with 3 teams.",
		"no money at all":        "We will sign the NDA by Friday.",
	}
	for name, words := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := Differing(words, dealMinor, "EUR"); ok {
				t.Errorf("%q flagged %d", words, got)
			}
		})
	}
}
