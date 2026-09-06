// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Where the period lands if nothing changes.
//
// The readings beside this one answer what is IN the pipeline. None of them
// answers the question a manager actually asks — "what will we finish on?" —
// and a reader left to add two of them together will pick the wrong two: Won
// plus Best case double-counts nothing but overstates, Weighted alone forgets
// the money already in the bank.
//
// Pure arithmetic over Readings. No clock, no database, no scope: the inputs
// are already scoped by the reader that produced them, and a projection that
// went looking for its own would answer about a different population than the
// figures it is reconciling against.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ForwardMeasure is which remaining-pipeline number a landing is built from,
// re-exported from the shared kernel.
//
// The set itself lives in values because identity stores the installation
// setting that names it and may not import this package. Aliased rather than
// re-declared: a second set here would be a second answer to the same question,
// and the two would drift the moment one gains a measure.
type ForwardMeasure = values.ForwardMeasure

const (
	// MeasureCommitEvidence is the strictest reading and the default.
	MeasureCommitEvidence = values.MeasureCommitEvidence
	// MeasureWeighted adds every open deal at its stage probability.
	MeasureWeighted = values.MeasureWeighted
	// MeasureManagerCall takes the authored call as the whole period's landing.
	MeasureManagerCall = values.MeasureManagerCall
)

// ForwardMeasures re-exports the shared kernel's set, so a caller holding this
// package need not import values to offer a chooser.
func ForwardMeasures() []ForwardMeasure { return values.ForwardMeasures() }

// Landing is where the period finishes, and what that answer rests on.
type Landing struct {
	// AmountMinor is the projection itself, in the base currency.
	AmountMinor int64
	// Measure is the one actually used, which is not always the one asked for:
	// a manager-call setting with no current call falls back and says so.
	Measure ForwardMeasure
	// WonMinor and RemainingMinor are the two halves a reconciliation line
	// prints. A manager call carries no split — it is a single authored total —
	// so RemainingMinor is zero there and the line says the call instead.
	WonMinor       int64
	RemainingMinor int64
	// Caveat names why this answer is not the plain one, empty when it is.
	Caveat LandingCaveat
}

// LandingCaveat is what a reader has to know about a projection to trust it.
type LandingCaveat string

const (
	// CaveatCallAbsent means a manager-call installation has no current call,
	// so the figure is the commit measure instead. Named rather than silently
	// substituted: a manager who set this expects their own number, and a
	// commit total wearing that label is the wrong number under a right word.
	CaveatCallAbsent LandingCaveat = "call_absent"
	// CaveatCallBelowActual means the authored call is less than the money
	// already won. It is reported rather than corrected: the call is somebody's
	// stated belief and this code does not get to overrule it, but a landing
	// below what is already banked is a number nobody should read past.
	CaveatCallBelowActual LandingCaveat = "call_below_actual"
)

// ProjectLanding answers where the period finishes.
//
// call is the current manager call for this period and scope, or nil. It is
// only consulted for MeasureManagerCall — a commit or weighted installation has
// made a different choice, and quietly preferring an authored number would make
// the setting a lie.
func ProjectLanding(readings Readings, measure ForwardMeasure, call *int64) (Landing, error) {
	switch measure {
	case MeasureCommitEvidence:
		return remainingLanding(readings, measure, readings.EvidenceMinor)
	case MeasureWeighted:
		return remainingLanding(readings, measure, readings.WeightedMinor)
	case MeasureManagerCall:
		if call == nil {
			// The commit measure, and SAID so. Falling back silently would show
			// a manager the number they did not ask for under the label of the
			// one they did.
			out, err := remainingLanding(readings, MeasureCommitEvidence, readings.EvidenceMinor)
			if err != nil {
				return Landing{}, err
			}
			out.Caveat = CaveatCallAbsent
			return out, nil
		}
		out := Landing{
			AmountMinor: *call, Measure: MeasureManagerCall,
			WonMinor: readings.WonMinor,
		}
		if *call < readings.WonMinor {
			out.Caveat = CaveatCallBelowActual
		}
		return out, nil
	default:
		return Landing{}, fmt.Errorf(
			"forecasting: %q is not a forward measure; a landing is built from commit evidence, "+
				"the weighted pipeline or a manager's own call", measure)
	}
}

// remainingLanding is Won plus what is still to come.
//
// Both remaining measures are REMAINING: readings.InOpen requires the deal is
// not won, so neither evidence nor weighted contains a deal already in the Won
// total. Adding them is therefore a sum of two disjoint sets rather than a
// double count — which is exactly why a manager call, a number for the WHOLE
// period, does not go through here.
// The sum goes through addMinor for the reason Compute's do: a headline that
// wrapped still looks like money and disagrees silently with the readings it
// claims to be the sum of. A landing is the one figure a manager repeats out
// loud, so it must refuse rather than wrap.
func remainingLanding(
	readings Readings, measure ForwardMeasure, remaining int64,
) (Landing, error) {
	total, err := addMinor(readings.WonMinor, remaining)
	if err != nil {
		return Landing{}, err
	}
	return Landing{
		AmountMinor:    total,
		Measure:        measure,
		WonMinor:       readings.WonMinor,
		RemainingMinor: remaining,
	}, nil
}
