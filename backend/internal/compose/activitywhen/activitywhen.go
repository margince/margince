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
	return occurredday.When(MomentOf(a))
}

// MomentOf is an activity's time as occurredday reads it.
func MomentOf(a crmcontracts.Activity) occurredday.Moment {
	return occurredday.Moment{At: a.OccurredAt, DateOnly: occurredday.IsDay(a.OccurredPrecision)}
}

// PrecisionOf is the wire precision of a time occurredday reads.
func PrecisionOf(m occurredday.Moment) *crmcontracts.OccurredPrecision {
	if m.DateOnly {
		return new(crmcontracts.OccurredPrecisionOccurredPrecisionDay)
	}
	return new(crmcontracts.OccurredPrecisionOccurredPrecisionInstant)
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
