// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Period buckets (year, quarter, month) over the governed run_report tool and
// the analytics door. Each report cuts them from its own date, in the
// installation's zone and financial year.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func setReportingZone(t *testing.T, e *Env, zone string) {
	t.Helper()
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone)
}

func setFiscalYearStart(t *testing.T, e *Env, month int) {
	t.Helper()
	e.WsExec(t, `INSERT INTO setting (key, value) VALUES ('installation.fiscal_year_start_month', to_jsonb($1::int))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, month)
}

// countsBy runs report grouped by one period grain and answers each bucket's
// count under the column the report's default names it.
func countsBy(ctx context.Context, t *testing.T, e *Env, report, grain, count string) map[string]string {
	t.Helper()
	plan := `{"group_by":["` + grain + `"],"aggregates":[{"fn":"count","as":"` + count + `"}]}`
	counts := map[string]string{}
	for _, row := range runPrebuiltReport(ctx, t, e, report, plan) {
		counts[cell(row, grain)] = cell(row, count)
	}
	return counts
}

func assertBuckets(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	// fmt prints a map in key order, so equal maps print alike.
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// Each pair sits half an hour either side of a Berlin midnight. A cut in UTC
// puts the second of each pair in the bucket before.
var (
	berlinNewYearsEve  = time.Date(2025, 12, 31, 22, 30, 0, 0, time.UTC)
	berlinNewYearsDay  = time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC)
	berlinLastOfMarch  = time.Date(2026, 3, 31, 21, 30, 0, 0, time.UTC)
	berlinFirstOfApril = time.Date(2026, 3, 31, 22, 30, 0, 0, time.UTC)
)

func TestAWinAtHalfPastElevenOnNewYearsEveInBerlinCountsInThatYear(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	company := seedRollupCompany(t, e, "Period Customer", &e.Rep1, nil)
	seedRollupWonDeal(t, e, st, company, 1_000, "EUR", "1.0", berlinNewYearsEve)
	seedRollupWonDeal(t, e, st, company, 1_000, "EUR", "1.0", berlinNewYearsDay)

	setReportingZone(t, e, "Europe/Berlin")
	assertBuckets(t, "won deals by year in Berlin", countsBy(e.Admin(), t, e, "win-loss", "period_year", "deals"),
		map[string]string{"2025": "1", "2026": "1"})
}

func TestActivitiesCountInThePeriodTheyHappenedInTheInstallationZone(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	for _, at := range []time.Time{berlinNewYearsEve, berlinNewYearsDay, berlinLastOfMarch, berlinFirstOfApril} {
		fileActivity(admin, t, e, "meeting", at, nil)
	}

	// The fixture's own discriminator: under UTC the pairs fall together.
	assertBuckets(t, "activities by year in UTC", countsBy(admin, t, e, "activities-by-kind", "period_year", "activities"),
		map[string]string{"2025": "2", "2026": "2"})

	setReportingZone(t, e, "Europe/Berlin")
	assertBuckets(t, "activities by year in Berlin", countsBy(admin, t, e, "activities-by-kind", "period_year", "activities"),
		map[string]string{"2025": "1", "2026": "3"})
	assertBuckets(t, "activities by quarter in Berlin", countsBy(admin, t, e, "activities-by-kind", "period_quarter", "activities"),
		map[string]string{"2025-Q4": "1", "2026-Q1": "2", "2026-Q2": "1"})
}

// A date-only entry is stored at noon UTC of its date. Read as an instant in a
// zone fourteen hours ahead, 31 December would land on 1 January.
func TestADateOnlyActivityCountsOnItsOwnDate(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	subject := "dated, not timed"
	date := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &date, OccurredPrecision: "day", Source: "manual",
	}); err != nil {
		t.Fatalf("logging a date-only meeting: %v", err)
	}

	setReportingZone(t, e, "Pacific/Kiritimati")
	assertBuckets(t, "a date-only meeting by month", countsBy(admin, t, e, "activities-by-kind", "period_month", "activities"),
		map[string]string{"2025-12": "1"})
}

func TestActivityPeriodsFollowTheFinancialYear(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	fileActivity(admin, t, e, "meeting", time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), nil)

	setFiscalYearStart(t, e, 4)
	for grain, want := range map[string]string{
		"period_year": "FY2025/26", "period_quarter": "FY2025/26-Q4", "period_month": "2026-03",
	} {
		assertBuckets(t, "a March meeting under an April start by "+grain,
			countsBy(admin, t, e, "activities-by-kind", grain, "activities"), map[string]string{want: "1"})
	}
}

// A task filed on a company only its owner may see counts for that owner. A
// colleague counts only the meeting both may see, in the same month.
func TestAnActivityPeriodCountsOnlyWhatTheReaderMaySee(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	hidden := seedRollupCompany(t, e, "Rep3's account", &e.Rep3, nil)
	when := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	fileActivity(admin, t, e, "meeting", when, nil)
	subject := "on the hidden account"
	if _, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "task", Subject: &subject, OccurredAt: &when, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: hidden}},
	}); err != nil {
		t.Fatalf("logging the task on the hidden account: %v", err)
	}
	e.MakeCapturePrivate(t, objCompany, hidden, e.Rep3)

	reader := func(seat ids.UUID, team ids.UUID) context.Context {
		return e.As(seat, []ids.UUID{team}, principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				objActivity: {Read: true}, objCompany: {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		})
	}
	assertBuckets(t, "Rep1's activities by month", countsBy(reader(e.Rep1, e.Team1), t, e, "activities-by-kind", "period_month", "activities"),
		map[string]string{"2026-05": "1"})
	assertBuckets(t, "Rep3's activities by month", countsBy(reader(e.Rep3, e.Team2), t, e, "activities-by-kind", "period_month", "activities"),
		map[string]string{"2026-05": "2"})
}

// keptYearCount asks how many activities fall in 2026 as the basis zone named
// it. Floor 1: a handful of meetings sit below the default floor, which would
// withhold the answer.
func keptYearCount(ctx context.Context, t *testing.T, e *Env, zone string) string {
	t.Helper()
	kept := analyticsquery.CalendarBasis{Timezone: zone, FiscalStartMonth: 1}
	q := analyticsquery.Query{
		Entity:   "activities-by-kind",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "activities"}},
		Filters:  []analyticsquery.Filter{{Field: "period_year", Op: analyticsquery.OpEq, Value: "2026", Basis: &kept}},
	}
	var answer compose.AnalyticsAnswer
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = compose.RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("asking for 2026 as %s named it: %v", zone, err)
	}
	if len(answer.Rows) != 1 {
		t.Fatalf("2026 as %s named it = %+v, want one row", zone, answer.Rows)
	}
	return cell(answer.Rows[0], "activities")
}

// A saved period filter keeps the dates it named under the zone it was saved
// in, for a timed meeting and a date-only one alike.
func TestAKeptActivityPeriodKeepsItsDatesWhenTheZoneMoves(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	fileActivity(admin, t, e, "meeting", berlinNewYearsEve, nil)
	fileActivity(admin, t, e, "meeting", berlinNewYearsDay, nil)
	subject := "dated, not timed"
	date := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &date, OccurredPrecision: "day", Source: "manual",
	}); err != nil {
		t.Fatalf("logging a date-only meeting: %v", err)
	}

	// Saved in Berlin, asked on this UTC installation: 2026 starts at Berlin's
	// midnight.
	if got := keptYearCount(admin, t, e, "Europe/Berlin"); got != "1" {
		t.Errorf("2026 as Berlin named it, asked under UTC = %s, want the one meeting after Berlin's midnight", got)
	}
	// 26 hours apart. 31 December begun at midnight twelve hours behind UTC is
	// already 2026 in Kiritimati, so only a date begun in the saved zone stays
	// in 2025.
	setReportingZone(t, e, "Etc/GMT+12")
	if got := keptYearCount(admin, t, e, "Pacific/Kiritimati"); got != "2" {
		t.Errorf("2026 as Kiritimati named it, asked twelve hours behind UTC = %s, want the two timed meetings and not the date-only 31 December", got)
	}
}
