// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Whether the team's open pipeline covered what the month and the quarter
// need, frozen at close.
//
// The wording is "coverage at risk against <basis>". The product has no target
// to be off, so a figure is only ever read against the basis it names.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// replanCoverageFloorBp is the coverage below which a replan is recommended:
// the pipeline covers under 80% of what the basis needs.
const replanCoverageFloorBp = 8000

// CoverageNotAuthorized says the acting member could not read the standing
// call, so "no coverage" and "could not ask" stay apart.
const CoverageNotAuthorized = "not_authorized"

// Coverage is one horizon's sufficiency, frozen. One of Basis and Absent is
// set, never both; the figures mean something only beside a Basis.
type Coverage struct {
	PeriodKind              string
	Basis                   string
	Absent                  string
	ReferenceLandingMinor   int64
	NeededOpenMinor         int64
	CurrentOpenMinor        int64
	RemainingToSupportMinor int64
	CoverageBp              int64
	BaseCurrency            string
	ReplanRecommended       bool
}

// TeamClose is what freezing the team's forecast hands back: the landing per
// horizon and the coverage per horizon, from one computation of the readings.
type TeamClose struct {
	Outlooks  []Outlook
	Movements []Movement
	Drivers   []Driver
	Coverage  []Coverage
}

// CoverageFrom freezes one sufficiency answer. The replan flag is raised only
// with a named basis, because a ratio against nothing recommends nothing.
func CoverageFrom(periodKind, baseCurrency string, s forecasting.Sufficiency) Coverage {
	if s.Absent != "" {
		return Coverage{PeriodKind: periodKind, Absent: string(s.Absent), BaseCurrency: baseCurrency}
	}
	return Coverage{
		PeriodKind: periodKind, Basis: string(s.Basis),
		ReferenceLandingMinor:   s.ReferenceLandingMinor,
		NeededOpenMinor:         s.NeededOpenMinor,
		CurrentOpenMinor:        s.CurrentOpenMinor,
		RemainingToSupportMinor: s.RemainingToSupportMinor,
		CoverageBp:              s.CoverageBp,
		BaseCurrency:            baseCurrency,
		ReplanRecommended:       s.CoverageBp < replanCoverageFloorBp,
	}
}

// CoverageRefused is the horizon whose standing call the acting member could
// not read.
func CoverageRefused(periodKind, baseCurrency string) Coverage {
	return Coverage{PeriodKind: periodKind, Absent: CoverageNotAuthorized, BaseCurrency: baseCurrency}
}

// ReplanRecommended reports whether any horizon of the week recommends one.
func (r TeamReview) ReplanRecommended() bool {
	for _, c := range r.Coverage {
		if c.ReplanRecommended {
			return true
		}
	}
	return false
}

// withheldFromReader removes what the reader may not read now from a frozen
// week. Coverage against a manager's call carries the call, which needs
// forecast read. A reader without it gets the horizon as not authorized. A
// reader who may not read meetings gets no meeting figure or pattern.
//
// summarise composes the summary again from what is left. One writer serves
// every reader, and none of them reads a withheld figure through a sentence.
func withheldFromReader(ctx context.Context, review *TeamReview) {
	forecast, meetings := auth.ReadGranted(ctx, "forecast"), MayReadMeetings(ctx)
	if forecast && meetings {
		return
	}
	if !forecast {
		for i, c := range review.Coverage {
			review.Coverage[i] = CoverageRefused(c.PeriodKind, c.BaseCurrency)
		}
	}
	if !meetings {
		withholdTeamMeetings(review)
	}
	if review.Summary != "" {
		review.Summary = summarise(*review)
	}
}
