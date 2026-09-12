// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// Both sides of the monthly reading answer the SAME corpus.
//
// The file this reads is also read by the browser bundle's own test, so a case
// added here is answered twice or one side goes red. Writing the cases out
// twice instead would prove only that the two lists agreed on the day somebody
// typed them, which is the arrangement the minor-unit vocabulary gate exists
// because of.

import (
	"encoding/json"
	"os"
	"testing"
)

type monthlyCase struct {
	Name        string `json:"name"`
	Currency    string `json:"currency"`
	AnnualMinor int64  `json:"annual_minor"`
	// MonthlyMinor and Approximate are what BOTH implementations must answer.
	MonthlyMinor int64 `json:"monthly_minor"`
	Approximate  bool  `json:"approximate"`
}

func TestMonthlyEquivalentAnswersTheSharedCorpus(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/monthly_equivalent.json")
	if err != nil {
		t.Fatalf("reading the shared corpus: %v", err)
	}
	var corpus struct {
		Cases []monthlyCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the shared corpus: %v", err)
	}
	// A corpus that parsed to nothing agrees with every implementation,
	// including a wrong one. The floor is what stops a green result meaning
	// that the file moved.
	if len(corpus.Cases) < 8 {
		t.Fatalf("the shared corpus holds %d cases — too few to have been read correctly, and a "+
			"test that checks nothing passes over any arithmetic at all", len(corpus.Cases))
	}

	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			monthly, approximate := MonthlyEquivalent(c.AnnualMinor)
			if monthly != c.MonthlyMinor {
				t.Errorf("MonthlyEquivalent(%d) = %d, want %d (%s)",
					c.AnnualMinor, monthly, c.MonthlyMinor, c.Currency)
			}
			if approximate != c.Approximate {
				t.Errorf("MonthlyEquivalent(%d) approximate = %v, want %v",
					c.AnnualMinor, approximate, c.Approximate)
			}
		})
	}
}

// Twelve months add back to the year exactly when the division said they would.
// This is the property the approximate flag claims, checked against the flag
// rather than restated as more cases.
func TestAnExactMonthlyReadingMultipliesBackToTheYear(t *testing.T) {
	t.Parallel()
	for annual := int64(0); annual < 500; annual++ {
		monthly, approximate := MonthlyEquivalent(annual)
		exact := monthly*12 == annual
		if exact == approximate {
			t.Fatalf("MonthlyEquivalent(%d) = %d reports approximate=%v, but twelve of it %s the year",
				annual, monthly, approximate, map[bool]string{true: "reproduce", false: "do not reproduce"}[exact])
		}
	}
}
