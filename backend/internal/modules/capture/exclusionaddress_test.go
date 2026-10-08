// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"strings"
	"testing"
)

// The contract caps `value` at 320 characters; a domain already stops short of
// that, so only the address arm could let a larger string into the table.
func TestAnAddressRuleLongerThanTheContractBoundIsRefusedNamingValue(t *testing.T) {
	t.Parallel()
	for _, length := range []int{321, 428, 5000} {
		raw := strings.Repeat("a", length-len("@example.com")) + "@example.com"
		_, err := ValidExclusionValue(ExclusionKindAddress, raw)
		var invalid *InvalidExclusionError
		if !errors.As(err, &invalid) || invalid.Field != "value" {
			t.Errorf("a %d-character address gave %v, want a refusal naming value", length, err)
		}
	}
}

func TestAnAddressRuleAtTheBoundIsKeptFolded(t *testing.T) {
	t.Parallel()
	raw := strings.Repeat("A", maxIndexedAddressChars-len("@example.com")) + "@Example.com"
	got, err := ValidExclusionValue(ExclusionKindAddress, raw)
	if err != nil || got != strings.ToLower(raw) {
		t.Fatalf("a %d-character address gave %q, %v; want it kept lowercased", len(raw), got, err)
	}
}
