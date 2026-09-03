// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// Why a question was not answered, in a form the asker can act on.
//
// A refusal that says only "no" costs a round trip and teaches nothing, and to
// an agent it costs several — it will rephrase and be refused again. So every
// refusal carries the SMALLEST clarification that would have worked: the names
// that exist, the measure that is allowed, the coarser grouping that clears the
// floor.
//
// The kinds are a closed set because a caller branches on them. A new kind is a
// new case every caller must handle, which is a decision rather than a detail.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// RefusalKind is why.
type RefusalKind string

const (
	// RefusalAmbiguous is a question with more than one reading. The
	// clarification names the readings.
	RefusalAmbiguous RefusalKind = "ambiguous"
	// RefusalInvalid is a question that means nothing: summing a stage name,
	// grouping by a measure.
	RefusalInvalid RefusalKind = "invalid"
	// RefusalUnsupported is a question this compiler cannot express. Distinct
	// from invalid, and the distinction matters to whoever reads it: invalid
	// will never work, unsupported might next quarter.
	RefusalUnsupported RefusalKind = "unsupported"
	// RefusalPrivacy is a question whose answer would describe too few people
	// or records.
	RefusalPrivacy RefusalKind = "privacy"
	// RefusalTooExpensive is a question the database would answer eventually.
	// Refused rather than run, because a query nobody cancelled is a query
	// still running when the next one arrives.
	RefusalTooExpensive RefusalKind = "too_expensive"
)

// RefusalError is a typed refusal.
type RefusalError struct {
	Kind RefusalKind
	// Message says what is wrong, in the asker's terms.
	Message string
	// Suggest is the smallest thing that would have worked. Never empty for a
	// refusal a caller could act on — a refusal without one is a dead end, and
	// this type exists so those are visible in review rather than shipped.
	Suggest string
}

func (e *RefusalError) Error() string {
	if e.Suggest == "" {
		return fmt.Sprintf("%s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("%s: %s — %s", e.Kind, e.Message, e.Suggest)
}

// Unwrap maps every refusal to ErrInvalidArgument.
//
// The HTTP layer then answers 422 for all of them, which is right: each is the
// caller's request being unanswerable as written, and none is a server fault or
// a permission denial. A privacy refusal is deliberately NOT a 403 — 403 would
// say the data exists and is withheld, and the point of the floor is to say
// nothing about a group that small.
func (e *RefusalError) Unwrap() error { return apperrors.ErrInvalidArgument }
