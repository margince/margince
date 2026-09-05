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
	// MeasureCommitEvidence adds the commit-category deals with confirmed close
	// dates. The strictest reading: a provisional date is a guess and stays out.
	MeasureCommitEvidence ForwardMeasure = "commit_evidence"
	// MeasureWeighted adds every open deal at its stage probability.
	MeasureWeighted ForwardMeasure = "weighted"
	// MeasureManagerCall takes the authored call as the whole period's landing.
	// It is NOT added to what is already won: a manager calling the quarter is
	// calling the finish line, not the distance left to run.
	MeasureManagerCall ForwardMeasure = "manager_call"
)

// ForwardMeasures is every measure, in the order a chooser should offer them:
// strictest first, then the two that soften it.
//
// Held by: TestEveryForwardMeasureIsOfferedOnTheWire (backend/gates/forwardmeasureparity_test.go),
// which derives the contract's own enums and requires them to agree with this
// set — a measure the settings screen admits and the projection refuses saves
// cleanly and then errors on every forecast read.
//
// Returned as a fresh slice because a package-level array would let one caller
// reorder every other caller's chooser.
func ForwardMeasures() []ForwardMeasure {
	return []ForwardMeasure{MeasureCommitEvidence, MeasureWeighted, MeasureManagerCall}
}

// ParseForwardMeasure turns a stored or wire value into a measure.
//
// The default is deliberate: an ABSENT measure is commit evidence, which is
// what every installation's landing was computed from before the setting
// existed, so one that never touches it sees no change.
func ParseForwardMeasure(raw *string, field string) (ForwardMeasure, error) {
	if raw == nil || *raw == "" {
		return MeasureCommitEvidence, nil
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
