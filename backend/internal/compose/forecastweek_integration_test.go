// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A forecast read over a WEEK.
//
// The week is the one window that is not a division of the financial year, so
// it is cut from the Monday weekly.WeekStartOf names rather than from month
// arithmetic. That crosses a seam two functions disagree about: WeekStartOf
// answers a calendar date stamped UTC, and ResolveWeek wants local midnight in
// the installation's zone. A unit test on either side passes while the route
// between them refuses every real request, which is what this covers.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestAWeekForecastResolvesThroughTheInstallationZone(t *testing.T) {
	e := integration.Setup(t)
	// A zone well east of UTC, so a Monday stamped UTC and a Monday stamped
	// locally are different instants and a route that confuses them fails.
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = '"Asia/Tokyo"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)

	var week forecasting.Period
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		got, _, err := ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodWeek, at)
		week = got
		return err
	}); err != nil {
		t.Fatalf("resolving the week: %v", err)
	}

	if week.StartDate.Weekday() != time.Monday {
		t.Fatalf("the week opened on a %s", week.StartDate.Weekday())
	}
	if week.Zone.String() != "Asia/Tokyo" {
		t.Fatalf("the week was cut in %s, not the installation's zone", week.Zone)
	}
	// The instant asked about is inside the window it was resolved for. This is
	// the assertion the zone bug fails: a week built from a UTC-stamped Monday
	// sits nine hours off in Tokyo, and an instant near a boundary falls out.
	if !week.ContainsInstant(at) {
		t.Fatalf("the week resolved for %s does not contain it — the window is offset from the day it was asked about",
			at.Format(time.RFC3339))
	}
	// Seven local days, inclusive at both ends.
	if got := week.EndDate.Sub(week.StartDate).Hours() / 24; got != 6 {
		t.Fatalf("the window spans %v days between its inclusive ends, wanted 6", got)
	}
}

// The two resolvers answer the SAME Monday.
//
// weekly.WeekStartOf is what the review and the weekly plan file work under, and
// the forecast has to agree with them or a week's readings and a week's review
// would be about different weeks with nothing saying so.
func TestTheForecastWeekIsTheWeekTheReviewIsAbout(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)

	var fromForecast, fromReview time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		week, _, err := ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodWeek, at)
		if err != nil {
			return err
		}
		fromForecast = week.StartDate
		monday, err := weekly.WeekStartOf(e.Admin(), tx, at)
		fromReview = monday
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if fromForecast.Format("2006-01-02") != fromReview.Format("2006-01-02") {
		t.Fatalf("the forecast's week opens %s and the review's opens %s — "+
			"one week, two answers, and nothing on either screen says which",
			fromForecast.Format("2006-01-02"), fromReview.Format("2006-01-02"))
	}
}

// A day named west of UTC is the day the caller wrote down.
//
// `as_of` is a `date`, which openapi parses as UTC midnight. In Los Angeles
// that instant is the previous local day, so asking about Monday the 1st was
// answered about the week that ended Sunday the 31st — a whole week early, with
// nothing on the page saying the window had moved.
func TestADayNamedWestOfUTCResolvesToItsOwnWeek(t *testing.T) {
	e := integration.Setup(t)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = '"America/Los_Angeles"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	// Monday 2026-06-01, exactly as the transport hands a date over — through
	// DayNamed, which is where a date stops being UTC midnight.
	asOf := forecasting.DayNamed(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	var week forecasting.Period
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		got, _, err := ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodWeek, asOf)
		week = got
		return err
	}); err != nil {
		t.Fatalf("resolving the week: %v", err)
	}

	if got := week.StartDate.Format("2006-01-02"); got != "2026-06-01" {
		t.Fatalf("asking about Monday 2026-06-01 answered the week opening %s — "+
			"a reader west of UTC is shown a week that already finished", got)
	}
}

// A reader asking with NO as_of gets the week they are standing in.
//
// The two inputs are different things and only one of them is a date. Monday
// 08:00 in Tokyo is Sunday 23:00 UTC, so a resolver that read every instant as
// a calendar date would answer about the week that just ended — the mirror of
// the west-of-UTC bug above, and the reason the date is anchored at the
// transport rather than guessed at in the seam.
func TestTheCurrentInstantResolvesToTheWeekTheReaderIsStandingIn(t *testing.T) {
	e := integration.Setup(t)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = '"Asia/Tokyo"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	// Monday morning there, Sunday night in UTC.
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, tokyo)

	var week forecasting.Period
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		got, _, err := ForecastPeriodAt(e.Admin(), tx, forecasting.PeriodWeek, now)
		week = got
		return err
	}); err != nil {
		t.Fatalf("resolving the week: %v", err)
	}

	if got := week.StartDate.Format("2006-01-02"); got != "2026-06-01" {
		t.Fatalf("Monday morning in Tokyo resolved to the week opening %s — "+
			"the reader is shown the week that just finished", got)
	}
}
