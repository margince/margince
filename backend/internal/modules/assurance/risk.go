// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// RiskSignalsVersion names the rules RiskSignals applies. A forecast frozen
// under another version measured other signals, so its effects are not pooled.
const RiskSignalsVersion = "v2"

// The risk signals a forecast weighs. Each is a fact about the deal, not a
// finding: no review is raised for one, and a deal may carry several.
const (
	SignalClosePushed = "close_pushed"
	SignalNoNextStep  = "no_next_step"
	SignalQuiet       = "quiet"
	SignalStuck       = "stuck"
)

// RiskSignalNames lists every signal RiskSignals can answer, in its order.
func RiskSignalNames() []string {
	return []string{SignalClosePushed, SignalNoNextStep, SignalQuiet, SignalStuck}
}

// RiskSignal is one signal with the facts behind it. close_pushed sets Count
// and ClosePassed; stuck sets Days and StageID; quiet sets Days; no_next_step
// sets ClosesInDays.
type RiskSignal struct {
	Code         string
	Days         *int
	Count        *int
	StageID      string
	ClosePassed  bool
	ClosesInDays *int
}

// soonDays is how near a close makes a missing next step worth naming the day.
const soonDays = 7

// RiskSignals answers which risk signals one open deal shows, in a fixed order.
// stuckAfter is how long nine in ten of this stage's won deals stayed in it.
// It is nil where too few won deals passed through. Days are counted by the
// calendar of loc, the installation's zone.
func RiskSignals(asOf time.Time, s Subject, cfg Config, stuckAfter *time.Duration, loc *time.Location) []RiskSignal {
	out := []RiskSignal{}
	closesIn := closeDaysFrom(asOf, s.ExpectedClose, loc)
	if closeKeepsMoving(s, cfg) {
		signal := RiskSignal{Code: SignalClosePushed, Count: new(s.CloseDatePushes)}
		signal.ClosePassed = closesIn != nil && *closesIn < 0
		out = append(out, signal)
	}
	if !s.HasNextStep {
		signal := RiskSignal{Code: SignalNoNextStep}
		if closesIn != nil && *closesIn >= 0 && *closesIn <= soonDays {
			signal.ClosesInDays = closesIn
		}
		out = append(out, signal)
	}
	if customerQuiet(s) {
		out = append(out, RiskSignal{Code: SignalQuiet, Days: new(s.Viability.SilentDays)})
	}
	if days, ok := stuckDays(asOf, s, stuckAfter, loc); ok {
		out = append(out, RiskSignal{Code: SignalStuck, Days: new(days), StageID: s.StageID})
	}
	return out
}

// stuckDays is how many calendar days the deal has sat in its stage, when that
// is past stuckAfter. Won deals clicked through a stage in seconds give a
// stuckAfter near zero, so a stint shorter than one day is never stuck.
func stuckDays(asOf time.Time, s Subject, stuckAfter *time.Duration, loc *time.Location) (int, bool) {
	if stuckAfter == nil || s.StageEnteredAt.IsZero() || asOf.Sub(s.StageEnteredAt) <= *stuckAfter {
		return 0, false
	}
	days := elapsed.Days(s.StageEnteredAt, asOf, loc)
	return days, days >= 1
}

// closeDaysFrom is how many calendar days after asOf the close date is, nil
// without one. The close is a date, so it is placed on loc's calendar as is.
func closeDaysFrom(asOf time.Time, closes *time.Time, loc *time.Location) *int {
	if closes == nil {
		return nil
	}
	day := time.Date(closes.Year(), closes.Month(), closes.Day(), 0, 0, 0, 0, loc)
	return new(elapsed.Days(asOf, day, loc))
}

// closeKeepsMoving is the close_pushed rule's own test. The review and the
// forecast signal both call it.
func closeKeepsMoving(s Subject, cfg Config) bool {
	return s.CloseDatePushes > cfg.MaxClosePushes
}

// customerQuiet reads the viability assessment: the customer has said nothing
// for longer than its threshold, or never spoke since the deal was created.
func customerQuiet(s Subject) bool {
	if s.Viability == nil {
		return false
	}
	return slices.Contains(s.Viability.Reasons, viability.ReasonCustomerSilent) ||
		slices.Contains(s.Viability.Reasons, viability.ReasonNeverContacted)
}
