// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"testing"
	"time"
)

// testYear is the year every fixture below sits in — the rules are about
// relative dates, so the year itself carries nothing.
const testYear = 2026

func at(m time.Month, d int) time.Time {
	return time.Date(testYear, m, d, 12, 0, 0, 0, time.UTC)
}

func ptrTime(t time.Time) *time.Time { return &t }
func money(v int64) *int64           { return &v }

// A deal with nothing wrong with it. Every case below varies ONE field, so what
// a case proves is that field.
func healthy() Subject {
	return Subject{
		DealID:           "d1",
		Owner:            "u1",
		AmountMinor:      money(500_000),
		Currency:         "EUR",
		ExpectedClose:    ptrTime(at(time.June, 30)),
		Category:         "commit",
		StageName:        "Negotiation",
		LastInboundAt:    ptrTime(at(time.May, 10)),
		NextStep:         "Send the revised terms",
		HasEconomicBuyer: true,
	}
}

// ask runs one rule by type, so a case names the rule it is about.
func ask(t *testing.T, ruleType string, asOf time.Time, s Subject, cfg Config) *Finding {
	t.Helper()
	for _, rule := range Rules() {
		if rule.Type == ruleType {
			return rule.Ask(asOf, s, cfg)
		}
	}
	t.Fatalf("no rule mints %q — the type and the rule are one-to-one", ruleType)
	return nil
}

// Every rule, with an admitting case and a refusing one.
//
// Both halves, always. A rule tested only on the case it fires for passes
// identically when it fires for EVERYTHING, and a pass that flags every deal is
// worse than one that flags none: it trains the reader to dismiss.
func TestEveryRuleAdmitsAndRefuses(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	asOf := at(time.May, 14)

	for _, tc := range []struct {
		ruleType string
		// fires is a deal the rule should notice.
		fires func(Subject) Subject
		// quiet is a deal it should say nothing about, beyond the healthy one.
		quiet func(Subject) Subject
	}{
		{
			ruleType: TypeClosePast,
			fires:    func(s Subject) Subject { s.ExpectedClose = ptrTime(at(time.April, 30)); return s },
			// Due TODAY has not gone by. The boundary, and the case a `<=`
			// would get wrong on the one day it matters most.
			quiet: func(s Subject) Subject { s.ExpectedClose = ptrTime(at(time.May, 14)); return s },
		},
		{
			ruleType: TypeCloseUnconfirmed,
			fires:    func(s Subject) Subject { s.CloseProvisional = true; return s },
			// Provisional but not committed: the forecast does not rest on it.
			quiet: func(s Subject) Subject {
				s.CloseProvisional, s.Category = true, "pipeline"
				return s
			},
		},
		{
			ruleType: TypeClosePushed,
			fires:    func(s Subject) Subject { s.CloseDatePushes = 3; return s },
			// Exactly at the threshold is within it.
			quiet: func(s Subject) Subject { s.CloseDatePushes = 2; return s },
		},
		{
			ruleType: TypeAmountVsOffer,
			fires:    func(s Subject) Subject { s.OfferTotalMinor = money(300_000); return s },
			// A gap below materiality is rounding somewhere, and a finding
			// about it costs a person's morning to dismiss.
			quiet: func(s Subject) Subject { s.OfferTotalMinor = money(499_990); return s },
		},
		{
			ruleType: TypeAmountVsContract,
			fires:    func(s Subject) Subject { s.ContractTotalMinor = money(200_000); return s },
			// No contract is not a disagreement with one.
			quiet: func(s Subject) Subject { s.ContractTotalMinor = nil; return s },
		},
		{
			ruleType: TypeNoNextStep,
			fires:    func(s Subject) Subject { s.NextStep = ""; return s },
			// No next step on a deal nobody is forecasting is not a finding.
			quiet: func(s Subject) Subject { s.NextStep, s.Category = "", "omitted"; return s },
		},
		{
			ruleType: TypeNoEconomicBuyer,
			fires:    func(s Subject) Subject { s.HasEconomicBuyer = false; return s },
			quiet:    func(s Subject) Subject { s.HasEconomicBuyer, s.Category = false, "pipeline"; return s },
		},
		{
			ruleType: TypeBuyerSilent,
			fires:    func(s Subject) Subject { s.LastInboundAt = ptrTime(at(time.January, 5)); return s },
			// Never heard from at all is a DIFFERENT finding, and not one this
			// rule claims: a deal created today has no inbound either.
			quiet: func(s Subject) Subject { s.LastInboundAt = nil; return s },
		},
		{
			ruleType: TypeCommitUnpriced,
			fires: func(s Subject) Subject {
				s.AmountMinor, s.Currency = nil, ""
				return s
			},
			quiet: func(s Subject) Subject {
				s.AmountMinor, s.Currency, s.Category = nil, "", "pipeline"
				return s
			},
		},
	} {
		t.Run(tc.ruleType, func(t *testing.T) {
			t.Parallel()
			if got := ask(t, tc.ruleType, asOf, tc.fires(healthy()), cfg); got == nil {
				t.Error("the rule said nothing about a deal it exists to notice")
			} else if got.Type != tc.ruleType {
				t.Errorf("the rule minted type %q, want %q — a rule that mints two types "+
					"gives one finding two identities", got.Type, tc.ruleType)
			}
			if got := ask(t, tc.ruleType, asOf, tc.quiet(healthy()), cfg); got != nil {
				t.Errorf("the rule flagged a deal it should be quiet about: %+v — a pass "+
					"that flags everything trains the reader to dismiss it", got)
			}
			if got := ask(t, tc.ruleType, asOf, healthy(), cfg); got != nil {
				t.Errorf("the rule flagged a healthy deal: %+v", got)
			}
		})
	}
}

// The identity is the type and the record, never the value.
//
// Keyed on the value, a close date that moved twice would be three exceptions
// and somebody would resolve the same finding three times.
func TestTheIdentityIgnoresTheObservedValue(t *testing.T) {
	t.Parallel()
	first := Finding{
		Type: TypeClosePast, SubjectID: "d1",
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	second := Finding{
		Type: TypeClosePast, SubjectID: "d1",
		Observed: map[string]any{"as_of": "2026-06-14"},
	}
	if LogicalKey(first) != LogicalKey(second) {
		t.Errorf("the same finding on two nights got two identities (%q, %q) — somebody "+
			"would resolve it twice", LogicalKey(first), LogicalKey(second))
	}
	// The admitting half: two DIFFERENT findings must not share one identity,
	// or resolving one silences the other.
	other := Finding{Type: TypeBuyerSilent, SubjectID: "d1"}
	if LogicalKey(first) == LogicalKey(other) {
		t.Error("two different exception types share one identity — resolving one would silence the other")
	}
	elsewhere := Finding{Type: TypeClosePast, SubjectID: "d2"}
	if LogicalKey(first) == LogicalKey(elsewhere) {
		t.Error("the same finding on two deals shares one identity")
	}
}

// The claim and the observed value hold structured values only. A frozen copy
// of a protected sentence outlives the protection on its source, which is how
// narrowing after birth stops being privacy.
func TestNoRuleCopiesFreeTextIntoAFinding(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	asOf := at(time.May, 14)

	// A deal whose every text field carries something a rule could be tempted
	// to quote back.
	loaded := healthy()
	loaded.NextStep = "Buyer said their CFO is on sick leave until October"
	loaded.StageName = "Negotiation — blocked on legal review of clause 7"
	loaded.ExpectedClose = ptrTime(at(time.April, 30))
	loaded.CloseProvisional = true
	loaded.CloseDatePushes = 5
	loaded.LastInboundAt = ptrTime(at(time.January, 5))
	loaded.HasEconomicBuyer = false
	loaded.OfferTotalMinor = money(100_000)
	loaded.ContractTotalMinor = money(90_000)

	for _, rule := range Rules() {
		found := rule.Ask(asOf, loaded, cfg)
		if found == nil {
			continue
		}
		for slot, values := range map[string]map[string]any{
			"claim": found.Claim, "observed": found.Observed,
		} {
			for key, value := range values {
				text, isText := value.(string)
				if !isText {
					continue
				}
				if text == loaded.NextStep || text == loaded.StageName {
					t.Errorf("%s copied the deal's free text into %s[%q]: %q — a frozen "+
						"copy of a protected sentence outlives the protection on its source",
						rule.Type, slot, key, text)
				}
			}
		}
	}
}
