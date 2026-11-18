// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"strings"
	"testing"
	"time"
)

var reviewedOn = time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

// citingRules is a reviewed pack that cites one law for one refusal.
func citingRules() Rules {
	return Rules{
		Jurisdiction: "de", Version: 1,
		Instruments: []Instrument{{
			Name: "Gesetz gegen den unlauteren Wettbewerb", Provision: "§7",
			SourceURL: "https://www.gesetze-im-internet.de/uwg_2004/__7.html",
		}},
		Provenance: Provenance{ReviewedAt: reviewedOn, Reviewer: "pack maintainers"},
		Citations: []Citation{{
			Code: "no_marketing_consent", Instrument: "Gesetz gegen den unlauteren Wettbewerb", Provision: "§7(2) Nr. 2",
		}},
	}
}

func TestAReviewedPackMayCiteADeclaredInstrument(t *testing.T) {
	if err := citingRules().Validate(); err != nil {
		t.Fatalf("a reviewed pack citing its own instrument was refused: %v", err)
	}
}

// Each case breaks one rule of the citation contract, so a validator that
// refused everything would not pass the case above and these together.
func TestACitationMustNameReviewedLawWithALinkableSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		breakIt func(*Rules)
	}{
		{"a draft rule cannot cite", func(r *Rules) { r.Provenance = Provenance{} }},
		{"an undeclared instrument", func(r *Rules) { r.Citations[0].Instrument = "Telemediengesetz" }},
		{"an http source", func(r *Rules) { r.Citations[0].SourceURL = "http://example.org/uwg" }},
		{"a script source", func(r *Rules) { r.Citations[0].SourceURL = "javascript:alert(1)" }},
		{"no source anywhere", func(r *Rules) { r.Instruments[0].SourceURL = "" }},
		{"no provision anywhere", func(r *Rules) { r.Instruments[0].Provision, r.Citations[0].Provision = "", "" }},
		{"an empty code", func(r *Rules) { r.Citations[0].Code = "" }},
		{"an unknown territory", func(r *Rules) { r.Citations[0].Territory = "sender_on_the_moon" }},
		{"a review naming nobody", func(r *Rules) { r.Provenance.Reviewer = "" }},
		{"a review due before it happened", func(r *Rules) { r.Provenance.ReviewDue = reviewedOn.AddDate(0, 0, -1) }},
		{"a second citation for every sender", func(r *Rules) {
			second := r.Citations[0]
			second.Territory = SenderElsewhere
			r.Citations = append(r.Citations, second)
		}},
		{"an ambiguous instrument", func(r *Rules) { r.Instruments = append(r.Instruments, r.Instruments[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := citingRules()
			r.Instruments = append([]Instrument(nil), r.Instruments...)
			r.Citations = append([]Citation(nil), r.Citations...)
			tc.breakIt(&r)
			if err := r.Validate(); err == nil {
				t.Errorf("accepted %s", tc.name)
			}
		})
	}
}

// Two territories of one code are two answers to two questions, and both stand.
func TestOneCodeMayCiteDifferentLawForASenderHereAndElsewhere(t *testing.T) {
	r := citingRules()
	here := r.Citations[0]
	here.Territory = SenderHere
	elsewhere := here
	elsewhere.Territory = SenderElsewhere
	r.Citations = []Citation{here, elsewhere}
	if err := r.Validate(); err != nil {
		t.Errorf("one code split by sender territory was refused: %v", err)
	}
}

func TestAnInstrumentSourceIsHTTPSAndItsEndFollowsItsStart(t *testing.T) {
	start := time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := (Instrument{Name: "Decree 91/2020/ND-CP", SourceURL: "ftp://example.org"}).Validate(); err == nil {
		t.Error("an ftp source was accepted")
	}
	if err := (Instrument{Name: "Decree 91/2020/ND-CP", EffectiveFrom: start, ExpiresFrom: start}).Validate(); err == nil {
		t.Error("an instrument ending as it begins was accepted")
	}
	if err := (Instrument{Name: "Decree 91/2020/ND-CP", EffectiveFrom: start, ExpiresFrom: start.AddDate(5, 0, 0)}).Validate(); err != nil {
		t.Errorf("an instrument with a later end was refused: %v", err)
	}
}

func TestADigestIsEmptyOrASHA256InHex(t *testing.T) {
	r := Rules{Jurisdiction: "de", Version: 1}
	r.Digest = strings.Repeat("0f", 32)
	if err := r.Validate(); err != nil {
		t.Errorf("a SHA-256 digest was refused: %v", err)
	}
	for _, bad := range []string{"abc", strings.Repeat("0F", 32), strings.Repeat("g", 64)} {
		r.Digest = bad
		if err := r.Validate(); err == nil {
			t.Errorf("digest %q was accepted", bad)
		}
	}
}

func TestResolvingACitationFillsWhatItLeftToItsInstrument(t *testing.T) {
	r := citingRules()
	got, err := r.ResolveCitation(Citation{Code: "x", Instrument: r.Instruments[0].Name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provision != "§7" || got.SourceURL != r.Instruments[0].SourceURL {
		t.Errorf("resolved to %+v, want the instrument's provision and source", got)
	}
}
