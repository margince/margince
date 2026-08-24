// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// A provider's confidence reaches this contract through `value_json`, which
// carries no CHECK. The column beside it is bounded 0..1 and is NOT where this
// number comes from — so between a vendor and a reader there was nothing at
// all.
//
// The contract declares the field `minimum: 0, maximum: 1`. A vendor that
// scores out of 100, which is a normal thing for a vendor to do, would put 87
// on it, and the page renders a confidence as a percentage.

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/ports/provider"
)

func phoneClaim(t *testing.T, confidence *float64) storedClaim {
	t.Helper()
	entry := map[string]any{"value": "+49 151 2345678"}
	if confidence != nil {
		entry["confidence"] = *confidence
	}
	raw, err := json.Marshal([]map[string]any{entry})
	if err != nil {
		t.Fatalf("encoding the fixture claim: %v", err)
	}
	return storedClaim{key: string(provider.ClaimMobilePhones), value: raw, provider: "surfe"}
}

func TestAConfidenceTheContractCannotCarryIsNotServed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		score *float64
		want  *float32
	}{
		{"a proportion is carried", providerPtr(0.87), providerPtr(float32(0.87))},
		{"zero is a real confidence, not an absent one", providerPtr(0.0), providerPtr(float32(0))},
		{"one is in range", providerPtr(1.0), providerPtr(float32(1))},
		{"a vendor scoring out of 100 states nothing here", providerPtr(87.0), nil},
		{"just over the bound is still over it", providerPtr(1.0000001), nil},
		{"a negative score is not a confidence", providerPtr(-0.5), nil},
		{"no score at all", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out crmcontracts.PersonProviderProfile
			if err := foldPhones(phoneClaim(t, tc.score), &out); err != nil {
				t.Fatalf("folding: %v", err)
			}
			if len(out.MobilePhones) != 1 {
				t.Fatalf("got %d phones, want the number itself to survive whatever the score was",
					len(out.MobilePhones))
			}
			got := out.MobilePhones[0].Confidence
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("served confidence %v for a score of %v — the contract declares this "+
					"field 0..1, and a page renders it as a percentage", *got, *tc.score)
			case tc.want != nil && got == nil:
				t.Errorf("dropped a confidence of %v, which the contract can carry", *tc.score)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("served %v, want %v", *got, *tc.want)
			}
		})
	}
}

// The number is what the row was bought for. However a vendor scales its
// certainty, the phone is unaffected — so an unstatable score must cost the
// confidence and nothing else.
func TestAnUnstatableScoreNeverCostsTheNumber(t *testing.T) {
	var out crmcontracts.PersonProviderProfile
	if err := foldPhones(phoneClaim(t, providerPtr(87.0)), &out); err != nil {
		t.Fatalf("folding: %v", err)
	}
	if len(out.MobilePhones) != 1 || out.MobilePhones[0].Value != "+49 151 2345678" {
		t.Errorf("phones = %+v — an out-of-range confidence took the number with it", out.MobilePhones)
	}
}
