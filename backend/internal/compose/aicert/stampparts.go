// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"crypto/sha256"
	"strings"
)

// StampParts says which of a scenario stamp's three digests moved. A stamp is
// three fixed-width hex digests concatenated (ScenarioStamps): the whole
// scenario, the request its site builds, and how a run is graded. So which one
// differs is read, not inferred.
type StampParts struct {
	// Case is true when the test itself changed: its fixture, its expected
	// answer, its rubric or its bands. Re-running measures a different question.
	Case bool
	// Prompt is true when the request this build sends changed while the test
	// stayed put. The band that follows describes the new prompt, and a drop is
	// a consequence of a product change rather than of the model.
	Prompt bool
	// Grader is true when how a run is graded changed: the judge's request or
	// the scoring rule. A band can move with neither the test nor the product
	// touched, which reads as a model regression and is not one.
	Grader bool
}

// Describe names the moved halves in the order a reader acts on them.
func (p StampParts) Describe() string {
	var moved []string
	if p.Case {
		moved = append(moved, "the case")
	}
	if p.Prompt {
		moved = append(moved, "the prompt this build sends")
	}
	if p.Grader {
		moved = append(moved, "how a run is graded")
	}
	if len(moved) == 0 {
		// A stamp this build cannot split: one written before the three-part
		// layout, or a length it does not recognise. Saying so beats a guess.
		return "something this build cannot attribute"
	}
	return strings.Join(moved, " and ")
}

// stampSegment is the width of each digest in a scenario stamp, and three of
// them is the whole stamp. Derived from sha256's hex width rather than written
// as 64, so a digest change cannot leave this reading the wrong slice.
const stampSegment = sha256.Size * 2

// splitStampChange compares two stamps segment by segment. An unrecognised
// length yields the zero value, whose Describe says it could not attribute the
// change. Under-reporting here must not look like an answer.
func splitStampChange(was, now string) StampParts {
	if len(was) != stampSegment*3 || len(now) != stampSegment*3 {
		return StampParts{}
	}
	return StampParts{
		Case:   was[:stampSegment] != now[:stampSegment],
		Prompt: was[stampSegment:stampSegment*2] != now[stampSegment:stampSegment*2],
		Grader: was[stampSegment*2:] != now[stampSegment*2:],
	}
}
