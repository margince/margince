// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The win-loss report and the period bucket it groups by (REPORT-KEY-8,
// REPORT-VOCAB-1, REPORT-PARAM-4..7). Three invariants earn their own tests
// because each one fails silently rather than loudly:
//
//   - A bucket must survive the derivation handle's round trip. The handle
//     carries a group key out through a URL query string and binds it back as
//     an equality predicate; a value that cannot be rendered and re-parsed
//     makes "Explain This Number" fail on exactly the reports that need it.
//   - A bucket boundary is the installation's, not UTC's. This one is
//     invisible on a UTC installation, which is most test fixtures.
//   - An OPEN deal is absent from win-loss, not a zero in it.

import (
	"encoding/json"
	"testing"
)

// wireFloat reads a cell that is legitimately fractional. A percentile
// interpolates between two rows, so its value is a decimal even where every
// input is a whole number of days — read as an integer it would truncate, and
// a median of 7.5 would assert equal to 7.
func wireFloat(t *testing.T, row map[string]any, key string) float64 {
	t.Helper()
	num, ok := row[key].(json.Number)
	if !ok {
		t.Fatalf("cell %q = %v (%T), want a number", key, row[key], row[key])
	}
	v, err := num.Float64()
	if err != nil {
		t.Fatalf("cell %q = %v: %v", key, num, err)
	}
	return v
}

// seedClosedDeal writes one won/lost deal closed at a given instant. Closed
// deals carry a frozen FX rate and a lost reason by CHECK constraint, so the
// helper supplies both rather than letting each test rediscover them.
func (e *forecastEnv) seedClosedDeal(t *testing.T, name, status, closedAt string, amountMinor int64) {
	t.Helper()
	e.seedClosedDealWith(t, name, status, "manual", closedAt, amountMinor)
}

// seedClosedDealFromSource writes a WON deal recorded under a given source,
// including the empty one a deal may legitimately carry (deal.source is NOT
// NULL with no CHECK against "").
func (e *forecastEnv) seedClosedDealFromSource(t *testing.T, name, source, closedAt string, amountMinor int64) {
	t.Helper()
	e.seedClosedDealWith(t, name, "won", source, closedAt, amountMinor)
}

func (e *forecastEnv) seedClosedDealWith(t *testing.T, name, status, source, closedAt string, amountMinor int64) {
	t.Helper()
	lostReason := "no reason"
	if status == "won" {
		lostReason = ""
	}
	var reason *string
	if lostReason != "" {
		reason = &lostReason
	}
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, status, closed_at, lost_reason, fx_rate_to_base, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'EUR', $6, $7::timestamptz, $8, 1.0, $9, 'human:x')`,
		name, e.pipeline, e.stages[60], amountMinor, status, closedAt, reason, source)
}

// setInstallationZone rewrites the reporting zone the period buckets are
// computed in. Seeded as UTC by setupForecast; a test that cares says so.
func (e *forecastEnv) setInstallationZone(t *testing.T, zone string) {
	t.Helper()
	if _, err := e.owner.Exec(t.Context(),
		`UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone); err != nil {
		t.Fatalf("setting the installation timezone: %v", err)
	}
}

// setFiscalYearStart moves the month the installation's business year begins.
//
// Seeded as January by setupForecast, the same way the zone is seeded as UTC —
// so this is a plain UPDATE like its sibling, and a statement that matched
// nothing would be a fixture that had stopped seeding the row.
func (e *forecastEnv) setFiscalYearStart(t *testing.T, month int) {
	t.Helper()
	tag, err := e.owner.Exec(t.Context(),
		`UPDATE setting SET value = to_jsonb($1::int)
		 WHERE key = 'installation.fiscal_year_start_month'`, month)
	if err != nil {
		t.Fatalf("setting the fiscal year start: %v", err)
	}
	// An UPDATE that matches nothing SUCCEEDS, and the test would then assert
	// against the January default while believing it had set something —
	// passing for the wrong reason, in the direction that hides a defect.
	if tag.RowsAffected() != 1 {
		t.Fatalf("the fiscal-year-start row is not seeded: UPDATE touched %d rows", tag.RowsAffected())
	}
}

// bucketRow finds the one aggregate row whose group keys all match, so a test
// naming two dimensions cannot silently assert against the wrong group.
func bucketRow(t *testing.T, result reportResultWire, keys map[string]string) map[string]any {
	t.Helper()
	for _, row := range result.Rows {
		matched := true
		for dimension, want := range keys {
			if row[dimension] != want {
				matched = false
				break
			}
		}
		if matched {
			return row
		}
	}
	t.Fatalf("no row matching %v in %+v", keys, result.Rows)
	return nil
}

func TestWinLossGroupsClosedDealsByYearAndExcludesOpenOnes(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "Won early", "won", "2025-03-04T10:00:00Z", 10000)
	e.seedClosedDeal(t, "Won late", "won", "2025-11-30T10:00:00Z", 25000)
	e.seedClosedDeal(t, "Won next year", "won", "2026-02-01T10:00:00Z", 7000)
	e.seedClosedDeal(t, "Lost", "lost", "2025-06-01T10:00:00Z", 90000)
	// An open deal has not been won or lost: absent from this report, never a
	// zero in it (REPORT-VOCAB-1's base set).
	e.seedOpenDeal(t, "Still open", 60, nil, int64p(500000), stringp("commit"))

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_year","status","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)

	won2025 := bucketRow(t, result, map[string]string{"period_year": "2025", "status": "won"})
	if got := wireInt(t, won2025, "deals"); got != 2 {
		t.Errorf("2025 won deals = %d, want 2", got)
	}
	if got := wireInt(t, won2025, "amount_minor_sum"); got != 35000 {
		t.Errorf("2025 won amount = %d, want 35000", got)
	}
	// Three groups: 2025/won, 2025/lost, 2026/won. The open deal makes no fourth.
	if len(result.Rows) != 3 {
		t.Errorf("rows = %d, want 3 (the open deal must not appear): %+v", len(result.Rows), result.Rows)
	}
}

// THE test for this feature: a period bucket must round-trip through the
// derivation handle. The bucket value leaves as a group key, travels through a
// URL query string, and comes back as an equality predicate against the same
// expression that produced it. A value that does not survive that trip makes
// every period report's "Explain This Number" fail at exactly the moment a
// reader stops trusting the figure and asks what is behind it.
func TestWinLossPeriodDerivationReconcilesExactly(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "A", "won", "2025-03-04T10:00:00Z", 10000)
	e.seedClosedDeal(t, "B", "won", "2025-11-30T10:00:00Z", 25000)
	e.seedClosedDeal(t, "Other year", "won", "2026-02-01T10:00:00Z", 7000)

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_year","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	row := bucketRow(t, result, map[string]string{"period_year": "2025"})
	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("the 2025 bucket has no derivation_url: %+v", row)
	}

	derivation := e.explainReport(e.Admin(), t, "win-loss", handle)
	if derivation.TotalRows != 2 || len(derivation.Rows) != 2 {
		t.Fatalf("drill-through = %d rows (total %d), want the bucket's 2 deals: %+v",
			len(derivation.Rows), derivation.TotalRows, derivation.Rows)
	}
	var sum int64
	for _, source := range derivation.Rows {
		if got := source["period_year"]; got != "2025" {
			t.Errorf("source row period_year = %v, want 2025 — the drill-through left the bucket", got)
		}
		sum += wireInt(t, source, "amount_minor")
	}
	if displayed := wireInt(t, row, "amount_minor_sum"); sum != displayed {
		t.Errorf("drill-through sum %d != displayed %d", sum, displayed)
	}
	// The recompute over the same rows must agree with the displayed cell too.
	if got := wireInt(t, derivation.Aggregates, "amount_minor_sum"); got != wireInt(t, row, "amount_minor_sum") {
		t.Errorf("recomputed aggregate %d != displayed %d", got, wireInt(t, row, "amount_minor_sum"))
	}
}

// A bucket boundary is the installation's, not UTC's (REPORT-PARAM-6). This
// deal closes 30 minutes into 2026 UTC, which is still 2025 in New York — so a
// report that ignored the zone would put it in the wrong year, and on a UTC
// installation nothing would ever notice.
func TestWinLossPeriodBucketsFollowTheInstallationZone(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "New year's eve in New York", "won", "2026-01-01T00:30:00Z", 10000)

	utc := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_year"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if got := utc.Rows[0]["period_year"]; got != "2026" {
		t.Fatalf("under UTC the bucket is %v, want 2026 — the fixture cannot discriminate", got)
	}

	e.setInstallationZone(t, "America/New_York")
	local := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_year"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if got := local.Rows[0]["period_year"]; got != "2025" {
		t.Errorf("under America/New_York the bucket is %v, want 2025 — the bucket ignored the installation zone", got)
	}
}

// A bucket's YEAR is the installation's business year, not the calendar's.
//
// This test is the one that holds the fiscal arithmetic, and it is here rather
// than in the deterministic lane because no deterministic test can do it. The
// mirror gate in backend/gates/frontendfiscalyear_test.go reads both spellings of the
// label out of source and checks their SHAPE; it cannot execute SQL assembled
// from bound tokens, so deleting the `- 1` from the month shift leaves every
// assertion there green while an April-start installation labels its year one
// year early.
//
// The instant is chosen so all three starts below disagree about it. 15 March
// 2026 is:
//
//	January start  → 2026        (the calendar year, unchanged)
//	April start    → FY2025/26   (the last month of the year that began in April 2025)
//	February start → FY2026/27   (the first quarter of the year that began weeks ago)
//
// A February start is deliberate. April, July and October are the fiscal starts
// one reaches for first, and every one of them is ALSO a calendar-quarter
// boundary — so a cut that ignored the setting entirely returns the same
// quarter and the case passes. The sibling unit test's first four cases were
// written that way and all four passed with the fiscal arithmetic deleted.
func TestWinLossPeriodBucketsFollowTheInstallationFiscalYear(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "Mid-March", "won", "2026-03-15T12:00:00Z", 10000)

	bucket := func(t *testing.T, dimension string) any {
		t.Helper()
		result := e.runReport(e.Admin(), t, "win-loss",
			`{"group_by":["`+dimension+`"],"aggregates":[{"fn":"count","as":"deals"}]}`)
		if len(result.Rows) != 1 {
			t.Fatalf("%s: %d rows, want the 1 seeded deal: %+v", dimension, len(result.Rows), result.Rows)
		}
		return result.Rows[0][dimension]
	}

	// The default first, and the fixture's own discriminator: if January did
	// not produce the plain calendar year, the cases below would prove nothing
	// about the fiscal branch.
	if got := bucket(t, "period_year"); got != "2026" {
		t.Fatalf("on the January default the year is %v, want the plain 2026", got)
	}
	if got := bucket(t, "period_quarter"); got != "2026-Q1" {
		t.Fatalf("on the January default the quarter is %v, want the plain 2026-Q1", got)
	}

	e.setFiscalYearStart(t, 4)
	if got := bucket(t, "period_year"); got != "FY2025/26" {
		t.Errorf("under an April start the year is %v, want FY2025/26 — March belongs to the year that began the previous April", got)
	}
	if got := bucket(t, "period_quarter"); got != "FY2025/26-Q4" {
		t.Errorf("under an April start the quarter is %v, want FY2025/26-Q4", got)
	}

	e.setFiscalYearStart(t, 2)
	if got := bucket(t, "period_year"); got != "FY2026/27" {
		t.Errorf("under a February start the year is %v, want FY2026/27", got)
	}
	if got := bucket(t, "period_quarter"); got != "FY2026/27-Q1" {
		t.Errorf("under a February start the quarter is %v, want FY2026/27-Q1 — February to April is its FIRST quarter", got)
	}

	// The month grain is not fiscal: a month is the same month under every
	// start, so the setting that just moved the other two must leave it alone.
	if got := bucket(t, "period_month"); got != "2026-03" {
		t.Errorf("the month grain is %v, want 2026-03 — it moved with the fiscal start", got)
	}
}

// The month SHIFT itself, on the one instant that can see it.
//
// Separate from the test above because the instants differ and the difference
// is the whole point. 15 March sits four months from an April boundary, so a
// shift that is off by one month still lands in the same fiscal year and the
// assertion passes over a broken expression — which it did: the reviewer's
// counter-example (dropping the `- 1`) survived the test above untouched.
//
// 1 April is the FIRST day of an April-start year, so being one month out puts
// it in the previous year's last quarter instead of this year's first. Correct
// gives FY2026/27-Q1; a shift of `months => 4` rather than `4 - 1` gives
// FY2025/26-Q4. Nothing else in the tree distinguishes those.
func TestTheFiscalMonthShiftIsExactAtTheYearsFirstDay(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "First day of the fiscal year", "won", "2026-04-01T12:00:00Z", 10000)
	e.setFiscalYearStart(t, 4)

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_quarter"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if len(result.Rows) != 1 {
		t.Fatalf("%d rows, want the 1 seeded deal: %+v", len(result.Rows), result.Rows)
	}
	if got := result.Rows[0]["period_quarter"]; got != "FY2026/27-Q1" {
		t.Errorf("the quarter is %v, want FY2026/27-Q1 — 1 April OPENS an April-start year, "+
			"and FY2025/26-Q4 means the month shift is one out", got)
	}
}

// The three grains are spelled canonically (REPORT-PARAM-5), and the spelling
// is load-bearing twice over: it is what a reader sees, and text in these
// formats sorts chronologically, so the report needs no separate sort key.
func TestWinLossPeriodGrainsUseTheirCanonicalSpelling(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "Q1", "won", "2026-02-11T10:00:00Z", 10000)
	e.seedClosedDeal(t, "Q4", "won", "2025-10-02T10:00:00Z", 10000)

	for dimension, want := range map[string][]string{
		"period_year":    {"2025", "2026"},
		"period_quarter": {"2025-Q4", "2026-Q1"},
		"period_month":   {"2025-10", "2026-02"},
	} {
		result := e.runReport(e.Admin(), t, "win-loss",
			`{"group_by":["`+dimension+`"],"aggregates":[{"fn":"count","as":"deals"}]}`)
		if len(result.Rows) != len(want) {
			t.Fatalf("%s: %d rows, want %d: %+v", dimension, len(result.Rows), len(want), result.Rows)
		}
		// Ordered by the dimension, so the returned order IS the assertion that
		// these values sort chronologically as text.
		for i, bucket := range want {
			if got := result.Rows[i][dimension]; got != bucket {
				t.Errorf("%s row %d = %v, want %q", dimension, i, got, bucket)
			}
		}
	}
}

// The two doors onto one question must answer it the same way. A response's own
// derivation handle spells an unset group key as the empty value, which the
// drill-through binds as IS NULL; the filter door used to bind `= NULL`, which
// is never true. So a report could answer "no rows" and hand the reader a link
// that answered "six" — from the same response, about the same question. The
// package's whole premise is that an explanation cannot disagree with the number
// it explains.
//
// Held by: TestAnUnsetFilterMeansTheSameThingOnBothDoors (backend/internal/compose/report_winloss_integration_test.go) — this test.
func TestAnUnsetFilterMeansTheSameThingOnBothDoors(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "Unowned A", "won", "2025-03-04T10:00:00Z", 10000)
	e.seedClosedDeal(t, "Unowned B", "won", "2025-11-30T10:00:00Z", 25000)

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"filters":{"owner_id":null},"aggregates":[{"fn":"count","as":"deals"}]}`)
	if len(result.Rows) == 0 {
		t.Fatalf("the filter door found nothing for owner_id=null, but both deals are unowned: %+v", result)
	}
	filtered := wireInt(t, result.Rows[0], "deals")

	derivation := e.explainReport(e.Admin(), t, "win-loss", result.DerivationURL)
	if got := wireInt(t, derivation.Aggregates, "deals"); got != filtered {
		t.Errorf("the filter door says %d and the handle it minted says %d — one question, two answers",
			filtered, got)
	}
}

// A year is spelled 2026 in JSON far more naturally than "2026", and the period
// grains are the first filters whose values look numeric. That call must come
// back as the caller's own mistake with the fix in it, never as an opaque 500
// that sends an agent retrying something that cannot succeed.
func TestANumericPeriodFilterIsRefusedAsTheCallersMistake(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "A", "won", "2026-03-04T10:00:00Z", 10000)

	if got := e.reportStatus(e.Admin(), "win-loss",
		`{"filters":{"period_year":2026},"aggregates":[{"fn":"count","as":"deals"}]}`); got != 422 {
		t.Errorf("a numeric period filter answered %d, want 422", got)
	}
	// The quoted spelling is the one that works, so the advice is actionable.
	if got := e.reportStatus(e.Admin(), "win-loss",
		`{"filters":{"period_year":"2026"},"aggregates":[{"fn":"count","as":"deals"}]}`); got != 200 {
		t.Errorf("the quoted spelling answered %d, want 200", got)
	}
}

// A group key that is EMPTY TEXT is not a group key that is ABSENT, and the
// handle has to keep them apart. `source` is the catalog's first unconstrained
// text dimension — every earlier one is a uuid, or carries a CHECK that forbids
// the empty string — so this is the first report where the two can collide.
// When they did, a bucket reported one deal and the handle it minted in the
// same response resolved to none.
func TestAnEmptyTextGroupKeyIsNotTheSameAsAnAbsentOne(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDealFromSource(t, "No source recorded", "", "2025-03-04T10:00:00Z", 10000)
	e.seedClosedDealFromSource(t, "Sourced", "manual", "2025-04-04T10:00:00Z", 20000)

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["source"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	row := bucketRow(t, result, map[string]string{"source": ""})
	displayed := wireInt(t, row, "deals")

	derivation := e.explainReport(e.Admin(), t, "win-loss", row["derivation_url"].(string))
	if got := wireInt(t, derivation.Aggregates, "deals"); got != displayed {
		t.Errorf("the empty-source bucket shows %d and its own handle resolves to %d — "+
			"empty text and absent collapsed to one spelling", displayed, got)
	}
	for _, source := range derivation.Rows {
		if source["source"] != "" {
			t.Errorf("drill-through row has source %q, want the empty-source deal", source["source"])
		}
	}
}

// amount_minor is a minor-unit integer in the deal's OWN currency, so a total
// that spans currencies is a number with no unit. The default plan is the one
// an agent calls first and a screen renders unattended, so it has to split.
func TestTheWinLossDefaultNeverSumsAcrossCurrencies(t *testing.T) {
	e := setupForecast(t)
	e.seedClosedDeal(t, "In euros", "won", "2025-03-04T10:00:00Z", 10000)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, status, closed_at, fx_rate_to_base, source, captured_by)
		VALUES ($1, 'In dollars', $2, $3, 10000, 'USD', 'won', now(), 1.0, 'manual', 'human:x')`,
		e.pipeline, e.stages[60])

	result := e.runReport(e.Admin(), t, "win-loss", `{}`)
	for _, row := range result.Rows {
		if row["currency"] == nil {
			t.Fatalf("a default row carries no currency, so its total has no unit: %+v", row)
		}
		if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
			t.Errorf("row %v totals %d — the two currencies were added together", row["currency"], got)
		}
	}
	if len(result.Rows) != 2 {
		t.Errorf("rows = %d, want one per currency: %+v", len(result.Rows), result.Rows)
	}
}

// seedDealTaking writes a WON deal that took a stated number of days, so a
// test can name the durations it expects a percentile over.
//
// created_at is set explicitly, which the other helpers leave to the column
// default. The measure under test is the difference between two dates, and a
// helper that fixed only one of them would let every deal take zero days.
func (e *forecastEnv) seedDealTaking(t *testing.T, name string, days int, closedAt string, amountMinor int64) {
	t.Helper()
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, status,
			created_at, closed_at, fx_rate_to_base, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'EUR', 'won',
			$6::timestamptz - make_interval(days => $7), $6::timestamptz, 1.0, 'manual', 'human:x')`,
		name, e.pipeline, e.stages[60], amountMinor, closedAt, days)
}

// How long a deal took is the other half of how a team is doing. The report
// knew how much closed and never how long it took, so a team getting slower at
// the same revenue looked identical to one that was not.
//
// Six deals so the group clears the engine's sample floor: 2, 4, 6, 8, 10 and
// 12 days. percentile_cont interpolates, so the median of six values is the
// midpoint of the third and fourth — 7 — rather than either of them.
func TestWinLossReportsHowLongDealsTookToClose(t *testing.T) {
	e := setupForecast(t)
	for i, days := range []int{2, 4, 6, 8, 10, 12} {
		e.seedDealTaking(t, "Took "+string(rune('A'+i)), days, "2025-06-15T10:00:00Z", 10000)
	}

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["status"],"aggregates":[{"fn":"count","as":"deals"},
			{"fn":"median","field":"days_to_close","as":"median_days"},
			{"fn":"p75","field":"days_to_close","as":"p75_days"}]}`)

	row := bucketRow(t, result, map[string]string{"status": "won"})
	if got := wireInt(t, row, "deals"); got != 6 {
		t.Fatalf("deals = %d, want the 6 seeded", got)
	}
	// The midpoint of 6 and 8. A median that came back as 6 or 8 would mean
	// the engine picked a row rather than interpolating, and the figure would
	// jump as deals were added rather than moving.
	if got := wireFloat(t, row, "median_days"); got != 7 {
		t.Errorf("median days = %v, want 7 — the midpoint of 6 and 8", got)
	}
	// percentile_cont at 0.75 over six sorted values interpolates at index
	// 3.75: 8 + 0.75 x (10 - 8).
	if got := wireFloat(t, row, "p75_days"); got != 9.5 {
		t.Errorf("p75 days = %v, want 9.5", got)
	}
}

// A deal created and closed on the same day took zero days, which is a fact
// about the sale rather than a missing figure.
func TestASameDayCloseTakesZeroDaysRatherThanNothing(t *testing.T) {
	e := setupForecast(t)
	// Five so the group clears the sample floor and the percentile is a
	// number rather than the NULL that would hide a wrong zero.
	for i := range 5 {
		e.seedDealTaking(t, "Same day "+string(rune('A'+i)), 0, "2025-06-15T10:00:00Z", 10000)
	}

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["status"],"aggregates":[{"fn":"count","as":"deals"},
			{"fn":"median","field":"days_to_close","as":"median_days"}]}`)

	row := bucketRow(t, result, map[string]string{"status": "won"})
	if got := wireFloat(t, row, "median_days"); got != 0 {
		t.Errorf("median days = %v, want 0 — a same-day close took no days", got)
	}
}

// A deal is aged in DAYS a person would count, not in elapsed clock time.
//
// Created at 23:50 and closed at 00:10 the next night, a deal has run over two
// calendar days and for 24 hours and 20 minutes. Counting the clock calls that
// one day; counting the calendar calls it two, and two is what a reader means
// by "it took two days" — the same reckoning stage age and the slipped rule
// already use.
//
// Six deals with the same span, so the group clears the sample floor and the
// median is that span rather than a NULL that would agree with either answer.
func TestADealIsAgedInCalendarDaysNotElapsedHours(t *testing.T) {
	e := setupForecast(t)
	for i := range 6 {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency,
				status, created_at, closed_at, fx_rate_to_base, source, captured_by)
			VALUES ($1, $2, $3, $4, 10000, 'EUR', 'won',
				'2025-06-14T23:50:00Z'::timestamptz, '2025-06-16T00:10:00Z'::timestamptz,
				1.0, 'manual', 'human:x')`,
			"Overnight "+string(rune('A'+i)), e.pipeline, e.stages[60])
	}

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["status"],"aggregates":[{"fn":"count","as":"deals"},
			{"fn":"median","field":"days_to_close","as":"median_days"}]}`)

	row := bucketRow(t, result, map[string]string{"status": "won"})
	if got := wireInt(t, row, "deals"); got != 6 {
		t.Fatalf("deals = %d, want the 6 seeded", got)
	}
	// 14 June to 16 June is two calendar days. Elapsed hours would say one.
	if got := wireFloat(t, row, "median_days"); got != 2 {
		t.Errorf("median days = %v, want 2 — 14 June to 16 June is two calendar "+
			"days, and 1 would mean the measure counts elapsed hours instead", got)
	}
}

// How long a deal took is counted on the INSTALLATION's clock, like the period
// bucket beside it.
//
// The same pair of instants spans a different number of local days in
// different zones: 23:50 to 00:10 two nights later is two days in UTC and one
// in Asia/Ho_Chi_Minh, where both fall inside a single local day. Read in the
// session's zone, the duration would move with the deployment region while the
// bucket on the very same row did not, and a row would disagree with itself.
func TestDaysToCloseFollowsTheInstallationZone(t *testing.T) {
	e := setupForecast(t)
	// Six identical deals, so the group clears the sample floor and the median
	// is the span itself rather than a NULL that would agree with any answer.
	for i := range 6 {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency,
				status, created_at, closed_at, fx_rate_to_base, source, captured_by)
			VALUES ($1, $2, $3, $4, 10000, 'EUR', 'won',
				'2025-06-14T23:50:00Z'::timestamptz, '2025-06-16T00:10:00Z'::timestamptz,
				1.0, 'manual', 'human:x')`,
			"Zoned "+string(rune('A'+i)), e.pipeline, e.stages[60])
	}
	plan := `{"group_by":["status"],"aggregates":[{"fn":"median","field":"days_to_close","as":"median_days"}]}`

	utc := e.runReport(e.Admin(), t, "win-loss", plan)
	if got := wireFloat(t, bucketRow(t, utc, map[string]string{"status": "won"}), "median_days"); got != 2 {
		t.Fatalf("under UTC the span is %v days, want 2 — the fixture cannot discriminate", got)
	}

	// +07: 23:50 UTC is 06:50 the NEXT morning locally, and 00:10 UTC two
	// nights later is 07:10 that same local day. One local day, not two.
	e.setInstallationZone(t, "Asia/Ho_Chi_Minh")
	local := e.runReport(e.Admin(), t, "win-loss", plan)
	if got := wireFloat(t, bucketRow(t, local, map[string]string{"status": "won"}), "median_days"); got != 1 {
		t.Errorf("under Asia/Ho_Chi_Minh the span is %v days, want 1 — the duration "+
			"ignored the installation zone while the period bucket beside it did not", got)
	}
}

// A deal that closed before it was created has no duration to report.
//
// Nothing in the schema orders the two columns, and the product's own write
// path cannot invert them — closed_at is set to now() and never taken from a
// caller — so this arrives through an import or a hand-written UPDATE. Left
// in, one backdated row drags a median toward a span nobody experienced, and
// a reader has no way to see it happen.
func TestABackdatedCloseHasNoDurationRatherThanANegativeOne(t *testing.T) {
	e := setupForecast(t)
	// Five honest deals of 10 days each, so the median is unambiguous, plus
	// one inverted row that must not move it.
	for i := range 5 {
		e.seedDealTaking(t, "Honest "+string(rune('A'+i)), 10, "2025-06-15T10:00:00Z", 10000)
	}
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency,
			status, created_at, closed_at, fx_rate_to_base, source, captured_by)
		VALUES ($1, $2, $3, $4, 10000, 'EUR', 'won',
			'2025-06-15T10:00:00Z'::timestamptz, '2025-06-05T10:00:00Z'::timestamptz,
			1.0, 'manual', 'human:x')`,
		"Closed before it existed", e.pipeline, e.stages[60])

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["status"],"aggregates":[{"fn":"count","as":"deals"},
			{"fn":"median","field":"days_to_close","as":"median_days"},
			{"fn":"min","field":"days_to_close","as":"min_days"}]}`)

	row := bucketRow(t, result, map[string]string{"status": "won"})
	// The row is still counted: it is a won deal, and the count is about deals
	// rather than about durations.
	if got := wireInt(t, row, "deals"); got != 6 {
		t.Errorf("deals = %d, want all 6 — the inverted row is still a won deal", got)
	}
	// No negative span reaches the figures.
	if got := wireFloat(t, row, "min_days"); got != 10 {
		t.Errorf("min days = %v, want 10 — a negative span reached the report", got)
	}
	if got := wireFloat(t, row, "median_days"); got != 10 {
		t.Errorf("median days = %v, want 10 — the inverted row moved the median", got)
	}
}
