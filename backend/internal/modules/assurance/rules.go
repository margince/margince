// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"fmt"
	"time"
)

// Subject is one deal as a rule sees it, already assembled by the seam.
//
// Deliberately not the deals module's own row: a rule is a question about a
// handful of fields, and taking the whole record would make every rule test
// need a deal writer to say anything about a close date.
type Subject struct {
	DealID string
	Owner  string
	// What the record claims. AmountMinor nil is an unpriced deal — real
	// pipeline with no number to check.
	AmountMinor *int64
	Currency    string
	// The close date the deal carries, and whether anybody confirmed it.
	ExpectedClose    *time.Time
	CloseProvisional bool
	Category         string
	StageName        string
	// The last time the BUYER said something, as distinct from the last time
	// anybody touched the record. A rep emailing a silent prospect weekly keeps
	// last_activity_at fresh while the buyer has said nothing for months.
	LastInboundAt *time.Time
	// The total of the most recent sent or accepted offer, when there is one.
	OfferTotalMinor *int64
	// The total of the signed contract, when there is one.
	ContractTotalMinor *int64
	// How many times the close date has been pushed within this period.
	CloseDatePushes int
	// Whether anybody wrote a next step — an open task or a booked upcoming
	// meeting on the deal. Existence only, deliberately: the step's own text is
	// activity content with an audience of its own, and no rule needs the words
	// to ask whether there are any.
	HasNextStep bool
	// Whether an economic buyer has been identified on the deal.
	HasEconomicBuyer bool
}

// Finding is what a rule noticed, before it becomes a stored exception.
//
// Claim and Observed hold STRUCTURED values only — a date, a minor-unit amount,
// an id. Never a snippet lifted from an activity body: a frozen copy of a
// protected sentence outlives the protection on its source, which is how
// narrowing after birth stops being privacy.
type Finding struct {
	Type      string
	SubjectID string
	Claim     map[string]any
	Observed  map[string]any
	Severity  string
	// How much money the finding puts in question. Nil where it cannot be said
	// — different from zero, which would claim nothing is at stake.
	AffectedMinor *int64
	Currency      string
}

// The exception types, and the one place they are named.
const (
	TypeClosePast        = "close_past"
	TypeCloseUnconfirmed = "close_unconfirmed"
	TypeClosePushed      = "close_pushed"
	TypeAmountVsOffer    = "amount_vs_offer"
	TypeAmountVsContract = "amount_vs_contract"
	TypeNoNextStep       = "no_next_step"
	TypeNoEconomicBuyer  = "no_economic_buyer"
	TypeBuyerSilent      = "buyer_silent"
	TypeCommitUnpriced   = "commit_unpriced"
)

// The forecast categories a rule asks about. Named because a category spelled
// four times can come to be spelled four ways, and a rule comparing against the
// wrong spelling is quietly never true.
const (
	categoryCommit   = "commit"
	categoryBestCase = "best_case"
)

// slotCategory is the claim key naming which category a finding is about.
const slotCategory = "category"

// Severities, ordered by how much a reader should drop to look.
const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

// Rule is one question asked of one deal.
//
// Pure: no clock of its own, no database. `asOf` arrives as the day the run is
// for, so a rule's answer is a function of its inputs and a test can state the
// day it means rather than depending on when it ran.
type Rule struct {
	// Type is the exception type this rule mints, and the two are one-to-one:
	// a rule that could mint two types would give one finding two identities
	// and a reader two things to resolve.
	Type string
	// Version changes when the rule's judgement changes. A finding appearing
	// or vanishing because a rule moved is not the business moving, and a
	// reader comparing two nights has to be able to tell.
	Version string
	Ask     func(asOf time.Time, s Subject, cfg Config) *Finding
}

// Config is what an installation decides rather than the product.
type Config struct {
	// SilentDays is how long a buyer may say nothing before it is worth
	// asking about. There is no segment-aware threshold today, and pretending
	// otherwise would be a setting nobody set.
	SilentDays int
	// MaterialMinor is the amount below which a discrepancy is not worth a
	// person's morning.
	MaterialMinor int64
	// MaxClosePushes is how many times a date may move before the date itself
	// is the finding rather than the moves.
	MaxClosePushes int
}

// DefaultConfig is what an installation gets before it decides otherwise.
func DefaultConfig() Config {
	return Config{SilentDays: 90, MaterialMinor: 100_000, MaxClosePushes: 2}
}

// Rules are the questions the nightly pass asks, in a fixed order.
//
// A slice rather than a map: the order is what a reader sees, and a map would
// reorder the findings between runs for no reason anybody chose.
//
// Held by: TestEveryAssuranceRuleAdmitsAndRefuses (gates/assurancerules_test.go),
// which derives the rule types from this file's own constants and fails when
// one is declared that no test exercises.
func Rules() []Rule {
	return []Rule{
		closePast(), closeUnconfirmed(), closePushed(),
		amountVsOffer(), amountVsContract(),
		noNextStep(), noEconomicBuyer(), buyerSilent(), commitUnpriced(),
	}
}

// A committed deal whose close date has already gone by. The commonest finding
// there is, and the one that quietly inflates a quarter: nobody moved the date,
// so the deal still counts toward a period it cannot close in.
func closePast() Rule {
	return Rule{Type: TypeClosePast, Version: "v1", Ask: func(asOf time.Time, s Subject, _ Config) *Finding {
		if s.ExpectedClose == nil || !s.ExpectedClose.Before(day(asOf)) {
			return nil
		}
		return &Finding{
			Type: TypeClosePast, SubjectID: s.DealID, Severity: SeverityHigh,
			Claim:         map[string]any{"expected_close": s.ExpectedClose.Format(time.DateOnly)},
			Observed:      map[string]any{"as_of": day(asOf).Format(time.DateOnly)},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// A committed deal whose date nobody confirmed. It is in the forecast on a
// guess, which is exactly what the evidence reading excludes and what a manager
// asking "are we sure" is asking about.
func closeUnconfirmed() Rule {
	return Rule{Type: TypeCloseUnconfirmed, Version: "v1", Ask: func(_ time.Time, s Subject, _ Config) *Finding {
		if s.Category != categoryCommit || !s.CloseProvisional {
			return nil
		}
		return &Finding{
			Type: TypeCloseUnconfirmed, SubjectID: s.DealID, Severity: SeverityMedium,
			Claim:         map[string]any{slotCategory: s.Category},
			Observed:      map[string]any{"close_provisional": true},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// A date that keeps moving. Each individual move is defensible; the pattern is
// the finding, and it is invisible to anyone looking at the deal today.
func closePushed() Rule {
	return Rule{Type: TypeClosePushed, Version: "v1", Ask: func(_ time.Time, s Subject, cfg Config) *Finding {
		if s.CloseDatePushes <= cfg.MaxClosePushes {
			return nil
		}
		return &Finding{
			Type: TypeClosePushed, SubjectID: s.DealID, Severity: SeverityMedium,
			Claim:         map[string]any{"max_pushes": cfg.MaxClosePushes},
			Observed:      map[string]any{"pushes": s.CloseDatePushes},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// The deal says one number and the offer that was sent says another. The buyer
// is looking at the offer.
func amountVsOffer() Rule {
	return Rule{Type: TypeAmountVsOffer, Version: "v1", Ask: func(_ time.Time, s Subject, cfg Config) *Finding {
		return amountDisagreement(TypeAmountVsOffer, s, s.OfferTotalMinor, "offer_total", cfg)
	}}
}

// The deal says one number and the signed contract says another. Worse than
// the offer case: this one has a signature on it.
func amountVsContract() Rule {
	return Rule{Type: TypeAmountVsContract, Version: "v1", Ask: func(_ time.Time, s Subject, cfg Config) *Finding {
		f := amountDisagreement(TypeAmountVsContract, s, s.ContractTotalMinor, "contract_total", cfg)
		if f != nil {
			f.Severity = SeverityHigh
		}
		return f
	}}
}

// amountDisagreement is the shared shape of the two money rules.
//
// Below the materiality threshold it says nothing: a discrepancy of a few cents
// is rounding somewhere, and a finding about it costs a person's morning to
// dismiss.
func amountDisagreement(kind string, s Subject, against *int64, slot string, cfg Config) *Finding {
	if s.AmountMinor == nil || against == nil {
		return nil
	}
	gap := *s.AmountMinor - *against
	if gap < 0 {
		gap = -gap
	}
	if gap < cfg.MaterialMinor {
		return nil
	}
	return &Finding{
		Type: kind, SubjectID: s.DealID, Severity: SeverityMedium,
		Claim:         map[string]any{"amount_minor": *s.AmountMinor},
		Observed:      map[string]any{slot: *against},
		AffectedMinor: &gap, Currency: s.Currency,
	}
}

// A deal in the forecast with nobody's next move written down. Not a
// bookkeeping complaint: a deal nobody can say the next step for is usually a
// deal that has stopped moving.
func noNextStep() Rule {
	return Rule{Type: TypeNoNextStep, Version: "v1", Ask: func(_ time.Time, s Subject, _ Config) *Finding {
		if s.Category != categoryCommit && s.Category != categoryBestCase {
			return nil
		}
		if s.HasNextStep {
			return nil
		}
		return &Finding{
			Type: TypeNoNextStep, SubjectID: s.DealID, Severity: SeverityLow,
			Claim:         map[string]any{slotCategory: s.Category},
			Observed:      map[string]any{"has_next_step": false},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// A committed deal with nobody identified who can sign. Late-stage deals
// without an economic buyer are where forecasts go to die.
func noEconomicBuyer() Rule {
	return Rule{Type: TypeNoEconomicBuyer, Version: "v1", Ask: func(_ time.Time, s Subject, _ Config) *Finding {
		if s.Category != categoryCommit || s.HasEconomicBuyer {
			return nil
		}
		return &Finding{
			Type: TypeNoEconomicBuyer, SubjectID: s.DealID, Severity: SeverityHigh,
			Claim:         map[string]any{slotCategory: s.Category},
			Observed:      map[string]any{"economic_buyer": false},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// The buyer has said nothing for a long time.
//
// INBOUND only. last_activity_at includes outbound mail, so a rep emailing a
// silent prospect weekly keeps that column fresh while the buyer has not
// answered in ninety days — and the deal looks alive to every screen reading
// it. This rule reads the buyer's own side.
func buyerSilent() Rule {
	return Rule{Type: TypeBuyerSilent, Version: "v1", Ask: func(asOf time.Time, s Subject, cfg Config) *Finding {
		if s.LastInboundAt == nil {
			// Nobody has ever heard from them. A different finding from
			// silence after a conversation, and not one this rule claims: a
			// deal created today has no inbound either.
			return nil
		}
		silentFor := int(day(asOf).Sub(day(*s.LastInboundAt)).Hours() / 24)
		if silentFor < cfg.SilentDays {
			return nil
		}
		return &Finding{
			Type: TypeBuyerSilent, SubjectID: s.DealID, Severity: SeverityMedium,
			Claim:         map[string]any{"silent_days_threshold": cfg.SilentDays},
			Observed:      map[string]any{"silent_days": silentFor},
			AffectedMinor: s.AmountMinor, Currency: s.Currency,
		}
	}}
}

// A committed deal with no price on it. It is counted as eligible pipeline and
// contributes nothing to the money, which is honest — and a commit nobody has
// priced is worth asking about.
func commitUnpriced() Rule {
	return Rule{Type: TypeCommitUnpriced, Version: "v1", Ask: func(_ time.Time, s Subject, _ Config) *Finding {
		if s.Category != categoryCommit || s.AmountMinor != nil {
			return nil
		}
		return &Finding{
			Type: TypeCommitUnpriced, SubjectID: s.DealID, Severity: SeverityMedium,
			Claim:    map[string]any{slotCategory: s.Category},
			Observed: map[string]any{"amount_minor": nil},
		}
	}}
}

// LogicalKey is an exception's stable identity: the type and the record it is
// about, never the value observed.
//
// Keyed on the value, a close date that moved twice would be three exceptions
// and somebody would resolve the same finding three times.
func LogicalKey(f Finding) string {
	return fmt.Sprintf("%s:%s", f.Type, f.SubjectID)
}

// day drops the clock. A rule comparing a date against an instant would answer
// differently at noon than at midnight on the same day.
func day(at time.Time) time.Time {
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
}
