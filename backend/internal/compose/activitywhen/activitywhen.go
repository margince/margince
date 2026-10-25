// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package activitywhen says when an activity happened, as a prompt states it.
// A brief or a draft then never shows a day entry's stored noon.
package activitywhen

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Text is an activity's time for a prompt: the date of a day entry and the
// RFC 3339 instant otherwise. A day's stored noon is no time anybody named.
func Text(a crmcontracts.Activity) string {
	return occurredday.When(a.OccurredAt, IsDay(a.OccurredPrecision))
}

// Stated is the wire precision of a row a reader found. It is nil when there
// was no row ("" from a SQL coalesce): a missing time has no precision.
func Stated(precision string) *crmcontracts.OccurredPrecision {
	if precision == "" {
		return nil
	}
	out := crmcontracts.OccurredPrecision(precision)
	return &out
}

// IsDay says a precision names only a date.
func IsDay(precision crmcontracts.OccurredPrecision) bool {
	return precision == crmcontracts.OccurredPrecisionOccurredPrecisionDay
}

// IsStatedDay is IsDay for a precision the wire may leave out, which it does
// only beside an absent time.
func IsStatedDay(precision *crmcontracts.OccurredPrecision) bool {
	return precision != nil && IsDay(*precision)
}
