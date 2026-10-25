// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package activitywhen says when an activity happened, as a prompt states it.
// A brief or a draft then never shows a day entry's stored noon.
package activitywhen

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Text is an activity's time for a prompt: the date of a day entry, read in the
// record zone, and the RFC 3339 instant otherwise. A day's stored noon is no
// time anybody named, so a prompt never sees it.
func Text(a crmcontracts.Activity, zone *time.Location) string {
	return occurredday.When(a.OccurredAt, IsDay(a.OccurredPrecision), zone)
}

// IsDay says a precision names only a date.
func IsDay(precision crmcontracts.OccurredPrecision) bool {
	return precision == crmcontracts.OccurredPrecisionOccurredPrecisionDay
}

// Zone reads the record zone for a builder that holds no transaction.
type Zone func(ctx context.Context) (*time.Location, error)

// Of resolves zone. Unbound is UTC, the zone setting's own default, which is
// what a builder in a unit test has; every shipped builder binds one.
func Of(ctx context.Context, zone Zone) (*time.Location, error) {
	if zone == nil {
		return time.UTC, nil
	}
	return zone(ctx)
}
