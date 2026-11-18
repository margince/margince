// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import (
	"slices"
	"testing"
	"time"
)

func base() Rules {
	return Rules{
		Jurisdiction: "aa", Version: 1,
		ReplyWindow:  30 * 24 * time.Hour,
		Disclosures:  []Disclosure{{Kind: PrivacyContact}, {Kind: ControllerIdentity}},
		FrequencyCap: &FrequencyCap{Messages: 3, Window: 24 * time.Hour},
		MarketingExceptions: []MarketingException{{
			Kind: ExistingCustomer, RequiresSaleEvidence: true,
		}},
		ClaimsNeedProof: []string{"best", "leading"},
	}
}

func TestALayerThatOnlyTightensLoosensNothing(t *testing.T) {
	tighter := base()
	tighter.Jurisdiction, tighter.Version = "bb", 7
	tighter.Instruments = []Instrument{{Name: "Act B"}}
	tighter.ReplyWindow = 7 * 24 * time.Hour
	tighter.Disclosures = []Disclosure{{Kind: ControllerIdentity}, {Kind: PrivacyContact}, {Kind: ObjectionRoute}}
	tighter.FrequencyCap = &FrequencyCap{Messages: 1, Window: 24 * time.Hour}
	tighter.MarketingExceptions = nil
	tighter.ConsentOnly = true
	tighter.ClaimsNeedProof = []string{"leading", "best", "unique"}
	if got := Loosened(base(), tighter); len(got) != 0 {
		t.Errorf("a stricter layer was reported as loosening %v", got)
	}
	if got := Loosened(base(), base()); len(got) != 0 {
		t.Errorf("a layer that restates the one below was reported as loosening %v", got)
	}
}

func TestEachLooserObligationIsNamed(t *testing.T) {
	for _, tc := range []struct {
		field  string
		loosen func(*Rules)
	}{
		{"ReplyWindow", func(r *Rules) { r.ReplyWindow = 365 * 24 * time.Hour }},
		{"ReplyWindow", func(r *Rules) { r.ReplyWindow = 0 }},
		{"Disclosures", func(r *Rules) { r.Disclosures = r.Disclosures[:1] }},
		{"FrequencyCap", func(r *Rules) { r.FrequencyCap = nil }},
		{"FrequencyCap", func(r *Rules) { r.FrequencyCap = &FrequencyCap{Messages: 10, Window: 24 * time.Hour} }},
		{"MarketingExceptions", func(r *Rules) {
			r.MarketingExceptions = []MarketingException{{Kind: ExistingCustomer, RequiresSimilarity: true}}
		}},
		{"ClaimsNeedProof", func(r *Rules) { r.ClaimsNeedProof = []string{"best"} }},
	} {
		looser := base()
		tc.loosen(&looser)
		if got := Loosened(base(), looser); !slices.Contains(got, tc.field) {
			t.Errorf("loosening %s was reported as %v", tc.field, got)
		}
	}
}

// A grant the layer below withholds is a looser layer: the fold keeps an
// exception only where both grant it.
func TestAnExceptionTheLayerBelowWithholdsLoosens(t *testing.T) {
	withholds := base()
	withholds.MarketingExceptions = nil
	if got := Loosened(withholds, base()); !slices.Equal(got, []string{"MarketingExceptions"}) {
		t.Errorf("a granted exception was reported as %v, want MarketingExceptions", got)
	}
}
