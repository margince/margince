// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// WHY the night picked a deal, in one word.
//
// Every brief entry used to reach the queue labelled "deal at risk", whatever
// the ranking had actually found. The scoring formula selects on winnability,
// value, timing, momentum and warmth — an attractive opportunity clears the bar
// on its own merits, and calling it drifting states a problem the evidence does
// not support. A rep who is told a healthy deal is at risk twice stops reading
// the label.
//
// Derived from the vector rather than stored beside it: the vector IS the
// reason, it is already persisted per item, and a second column would be a
// second answer that can disagree with the first.

// BriefSignal is why one deal is on the morning queue.
type BriefSignal string

const (
	// SignalStalled — the deal has not moved and nobody has heard from the
	// buyer. The label the whole queue used to wear.
	SignalStalled BriefSignal = "stalled"
	// SignalClosingSoon — a date is near enough that the work is now.
	SignalClosingSoon BriefSignal = "closing_soon"
	// SignalOpportunity — winnable and worth money, with nothing wrong. The
	// case the old label was flatly untrue about.
	SignalOpportunity BriefSignal = "opportunity"
	// SignalMoved — something happened on it since the rep last looked.
	SignalMoved BriefSignal = "moved"
)

// signalThresholds are the readings that make a factor worth naming.
//
// They are not the ranking's own weights and must not be read as tuning it:
// the composite already decided this deal is on the queue, and these decide
// which of the five factors to SAY. A deal enters the queue at 0.15 composite,
// so a factor at 0.6 is one that carried real weight in getting it there.
const (
	signalStrong = 0.6
	// A deal is timing-urgent well before it is winnable-strong: a date that
	// has nearly arrived is work today whatever the odds.
	signalUrgent = 0.75
)

// SignalOf names why this deal is here, reading the same vector the composite
// folded.
//
// ORDER IS THE JUDGEMENT, and it runs from most actionable to least. A deal
// that is both closing and quiet is closing — the date is the thing the rep can
// still act on, and "stalled" would file urgent work under a watchlist. A deal
// that is quiet and attractive is stalled, because the silence is the fact that
// needs answering before the opportunity means anything.
//
// The fallback is opportunity rather than stalled, and that is the correction
// this exists to make. A deal the night ranked highly with no urgency, no
// silence and no movement is one worth attention on its merits; labelling it
// at-risk describes a problem nobody found.
func SignalOf(v BriefFeatureVector) BriefSignal {
	switch {
	case v.Timing >= signalUrgent:
		return SignalClosingSoon
	case v.Momentum <= briefMomentumUnchanged && v.Warmth < signalStrong:
		// Nothing has happened AND the relationship is cold. Either alone is
		// ordinary — a warm deal between conversations is not stalling, and a
		// deal that moved yesterday is not quiet — so the pair is what makes
		// silence worth naming.
		return SignalStalled
	case v.Momentum > briefMomentumUnchanged:
		return SignalMoved
	default:
		return SignalOpportunity
	}
}
