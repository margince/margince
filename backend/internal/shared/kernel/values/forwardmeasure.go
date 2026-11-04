// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// Which remaining-pipeline reading a projected landing is built from.
//
// Spelled in this package rather than in forecasting because TWO modules need
// the set and they may not import each other: identity stores the installation
// setting and must validate it, forecasting builds the landing and must refuse
// anything else. A copy in either is a set that can drift from the one the
// other enforces — and the drift is invisible until an installation saves a
// measure the projection then refuses on every read.

import "fmt"

// ForwardMeasure names which reading the forward half of a landing comes from.
type ForwardMeasure string

const (
	// MeasureLikely adds the open deals at or above the Likely threshold with
	// confirmed close dates. The strictest reading: a provisional date is a
	// guess and stays out.
	MeasureLikely ForwardMeasure = "likely"
	// MeasureWeighted adds every open deal at its stage probability.
	MeasureWeighted ForwardMeasure = "weighted"
	// MeasureManagerCall takes the authored call as the whole period's landing.
	// It is NOT added to what is already won: a manager calling the quarter is
	// calling the finish line, not the distance left to run.
	MeasureManagerCall ForwardMeasure = "manager_call"
)

// ForwardMeasures is every measure, in the order a chooser should offer them:
// strictest first, then the manager call.
//
// Held by: TestEveryForwardMeasureIsOfferedOnTheWire (backend/gates/forwardmeasureparity_test.go),
// which derives the contract's own enums and requires them to agree with this
// set — a measure the settings screen admits and the projection refuses saves
// cleanly and then errors on every forecast read.
//
// Returned as a fresh slice because a package-level array would let one caller
// reorder every other caller's chooser.
func ForwardMeasures() []ForwardMeasure {
	return []ForwardMeasure{MeasureLikely, MeasureWeighted, MeasureManagerCall}
}

// legacyForwardMeasures are the spellings stored rows still hold: snapshots
// frozen before Likely replaced commit evidence, and measures of the retired
// qualified forecast, which read as the baseline reading they restricted.
var legacyForwardMeasures = map[string]ForwardMeasure{
	"commit_evidence":           MeasureLikely,
	"qualified_commit_evidence": MeasureLikely,
	"qualified_likely":          MeasureLikely,
	"qualified_weighted":        MeasureWeighted,
}

// ParseForwardMeasure turns a stored or wire value into a measure.
//
// An absent measure is Likely, the strictest reading, so an installation
// that never touches the setting sees no change.
func ParseForwardMeasure(raw *string, field string) (ForwardMeasure, error) {
	if raw == nil || *raw == "" {
		return MeasureLikely, nil
	}
	if measure, ok := legacyForwardMeasures[*raw]; ok {
		return measure, nil
	}
	for _, known := range ForwardMeasures() {
		if ForwardMeasure(*raw) == known {
			return known, nil
		}
	}
	return "", &ParseError{
		Field: field, Code: "unknown_forward_measure",
		Message: fmt.Sprintf("a landing is built from one of %v", ForwardMeasures()),
	}
}
