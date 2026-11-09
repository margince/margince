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

import (
	"fmt"
	"slices"
)

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

// ForwardMeasures is every measure, strictest first, as a chooser offers them.
// A fresh slice, so no caller can reorder another's chooser.
//
// Held by: TestEveryForwardMeasureIsOfferedOnTheWire (backend/gates/forwardmeasureparity_test.go).
func ForwardMeasures() []ForwardMeasure {
	return []ForwardMeasure{MeasureLikely, MeasureWeighted, MeasureManagerCall}
}

// retiredQualifiedLikely is the retired qualified reading of MeasureLikely.
const retiredQualifiedLikely = "qualified_likely"

// RetiredForwardMeasures are the measures of the retired qualified forecast. A
// frozen weekly review keeps the word it was read under; nothing builds them.
func RetiredForwardMeasures() []string {
	return []string{retiredQualifiedLikely, "qualified_weighted"}
}

// SavedForwardMeasure is what a settings patch stores for a sent measure: a
// retired measure's base measure, and any other value as it came.
func SavedForwardMeasure(sent string) string {
	if slices.Contains(RetiredForwardMeasures(), sent) {
		return string(legacyForwardMeasures[sent])
	}
	return sent
}

// RetiredForwardMeasure names the retired measure a stored spelling means, the
// older qualified_commit_evidence included.
func RetiredForwardMeasure(raw string) (string, bool) {
	if raw == "qualified_commit_evidence" {
		return retiredQualifiedLikely, true
	}
	return raw, slices.Contains(RetiredForwardMeasures(), raw)
}

// legacyForwardMeasures are the spellings stored rows still hold. A qualified
// measure reads as the baseline reading it restricted.
var legacyForwardMeasures = map[string]ForwardMeasure{
	"commit_evidence":           MeasureLikely,
	"qualified_commit_evidence": MeasureLikely,
	retiredQualifiedLikely:      MeasureLikely,
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
