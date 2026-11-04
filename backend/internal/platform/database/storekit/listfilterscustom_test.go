// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"testing"
)

// customListInput is listInput with the custom-filter input a store fills from
// cf_ query parameters.
type customListInput struct {
	listInput
	Custom map[string]string
}

var probeCustomFilters = FilterSet[customListInput]{
	"status": FilterWord(func(in *customListInput, v *string) { in.Status = v }),
}

func setProbeCustom(in *customListInput, custom map[string]string) { in.Custom = custom }

func TestApplyCustomSetsTheCfKeysAsideAndBindsTheRest(t *testing.T) {
	var in customListInput
	if err := probeCustomFilters.ApplyCustom(&in, map[string]string{
		"status": "open", "cf_region": "North",
	}, setProbeCustom); err != nil {
		t.Fatalf("applying a core and a custom filter: %v", err)
	}
	if in.Status == nil || *in.Status != "open" {
		t.Errorf("status bound %v, want open", in.Status)
	}
	if len(in.Custom) != 1 || in.Custom["cf_region"] != "North" {
		t.Errorf("the custom filters landed as %v, want cf_region=North alone", in.Custom)
	}
}

// The bare prefix names no field, so it is an unknown filter like any other.
func TestApplyCustomRefusesAnUnknownCoreNameAndLeavesTheInput(t *testing.T) {
	var in customListInput
	err := probeCustomFilters.ApplyCustom(&in, map[string]string{"cf_": "x", "cf_region": "North"}, setProbeCustom)
	var refused *FilterError
	if !errors.As(err, &refused) || refused.Name != "cf_" {
		t.Fatalf("want the bare prefix refused by name, got %v", err)
	}
	if in.Custom != nil {
		t.Errorf("a refused call set the custom filters to %v", in.Custom)
	}
}

func TestApplyCustomWithNoCfKeysLeavesTheCustomInputUnset(t *testing.T) {
	var in customListInput
	if err := probeCustomFilters.ApplyCustom(&in, map[string]string{"status": "open"}, setProbeCustom); err != nil {
		t.Fatalf("applying a core filter: %v", err)
	}
	if in.Custom != nil {
		t.Errorf("no cf_ key, yet the custom filters are %v", in.Custom)
	}
}
