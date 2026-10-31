// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// RiskSignalsVersion names the rules RiskSignals applies. A forecast frozen
// under another version measured other signals, so its effects are not pooled.
const RiskSignalsVersion = "v1"

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

// RiskSignals answers which risk signals one open deal shows, in a fixed order.
// stuckAfter is how long nine in ten of this stage's won deals stayed in it,
// as the seam measured it; nil where too few won deals passed through.
func RiskSignals(asOf time.Time, s Subject, cfg Config, stuckAfter *time.Duration) []string {
	out := []string{}
	if closeKeepsMoving(s, cfg) {
		out = append(out, SignalClosePushed)
	}
	if !s.HasNextStep {
		out = append(out, SignalNoNextStep)
	}
	if customerQuiet(s) {
		out = append(out, SignalQuiet)
	}
	if stuckAfter != nil && !s.StageEnteredAt.IsZero() && asOf.Sub(s.StageEnteredAt) > *stuckAfter {
		out = append(out, SignalStuck)
	}
	return out
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
