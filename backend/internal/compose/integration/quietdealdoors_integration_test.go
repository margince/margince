// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The quiet-deal filters on every door. The schema route offers them typed, and
// the typed query, the report route and both tools agree on one answer.
// The day count is the installation's calendar, judged at injected instants.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/analyticswire"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/quietfilters"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// quietFilterShapes is what the schema must say about each filter.
var quietFilterShapes = []struct{ name, shape string }{
	{"days_since_activity", "number"}, {"meeting_ahead", "boolean"}, {"company_days_since_activity", "number"},
}

func filterShapesOf(entity analyticsquery.Entity) map[string]string {
	out := map[string]string{}
	if filters := analyticswire.FiltersOf(entity); filters != nil {
		for _, f := range *filters {
			out[f.Name] = string(f.Shape)
		}
	}
	return out
}

// The schema route lists the three filters with the shape of the value each
// takes. The vocabulary a seat without the activity grant is handed leaves the
// meeting filter out.
func TestTheSchemaOffersTheQuietFiltersTyped(t *testing.T) {
	d := newTwoDoors(t, "quiet-schema", "read")
	var schema struct {
		Entities []struct {
			Name    string `json:"name"`
			Filters []struct {
				Name  string `json:"name"`
				Shape string `json:"shape"`
			} `json:"filters"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/analytics/schema", nil), &schema); err != nil {
		t.Fatalf("the schema is not the contract's shape: %v", err)
	}
	for _, entity := range schema.Entities {
		if entity.Name != "deals-by-stage" {
			continue
		}
		got := map[string]string{}
		for _, f := range entity.Filters {
			got[f.Name] = f.Shape
		}
		for _, want := range quietFilterShapes {
			if got[want.name] != want.shape {
				t.Errorf("the schema offers %s as %q, want %q (all filters: %v)", want.name, got[want.name], want.shape, got)
			}
		}
		return
	}
	t.Fatal("the schema route offers no deals-by-stage population")
}

func TestTheMeetingFilterIsLeftOutOfTheVocabularyWithoutTheActivityGrant(t *testing.T) {
	e := Setup(t)
	for object, wantMeeting := range map[string]bool{"": true, "activity": false} {
		var objects []string
		if object != "" {
			objects = append(objects, object)
		}
		ctx := quietSeat(e, objects...)
		var schema analyticsquery.Schema
		err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			schema, err = compose.AnalyticsSchemaWithCoverage(ctx, tx)
			return err
		})
		if err != nil {
			t.Fatalf("reading the vocabulary: %v", err)
		}
		shapes := filterShapesOf(schema.Entities["deals-by-stage"])
		if _, offered := shapes["meeting_ahead"]; offered != wantMeeting {
			t.Errorf("without %q the vocabulary offers meeting_ahead = %v, want %v", object, offered, wantMeeting)
		}
		if shapes["days_since_activity"] != "number" {
			t.Errorf("without %q days_since_activity is %q, want a number", object, shapes["days_since_activity"])
		}
	}
}

// touchDeal files a note on a deal some days ago, through the activity triggers.
func touchDeal(t *testing.T, d *twoDoors, deal string, daysAgo int) {
	t.Helper()
	activity := ids.NewV7()
	ctx := context.Background()
	if _, err := d.e.Owner.Exec(ctx, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'Touch', now() - make_interval(days => $2), 'manual', 'human:x')`, activity, daysAgo); err != nil {
		t.Fatalf("filing the note: %v", err)
	}
	if _, err := d.e.Owner.Exec(ctx, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), activity, deal); err != nil {
		t.Fatalf("linking the note: %v", err)
	}
}

// dealsPerAge keeps every group above the privacy floor, which refuses a filter
// that singles out a handful of records.
const dealsPerAge = 6

// "At least 30 days quiet" is one question on every door: the typed query, the
// report route and the two tools. Deals touched 5, 30 and 45 days ago and some
// never touched are worth 1, 2, 4 and 8, so the sum names the deals counted.
func TestQuietDealsAreOneAnswerOnEveryDoor(t *testing.T) {
	d := newTwoDoors(t, "quiet-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for i, daysAgo := range []int{5, 30, 45, -1} {
		for range dealsPerAge {
			deal := createdID(t, d.e, "/v1/deals", AnyMap{
				"name": fmt.Sprint("Quiet ", daysAgo), "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
				"source": "manual", "amount_minor": 1 << i, "currency": "EUR",
			})
			if daysAgo >= 0 {
				touchDeal(t, d, deal, daysAgo)
			}
		}
	}
	const want = dealsPerAge * (2 + 4 + 8)

	query := map[string]any{
		"entity": "deals-by-stage", "group_by": []any{"currency"}, "scope_kind": "workspace",
		"measures": []any{map[string]any{"fn": "sum", "field": "amount_minor", "as": "amount"}},
		"filters":  []any{map[string]any{"field": "days_since_activity", "op": "gte", "value": 30}},
	}
	report := map[string]any{
		"group_by":   []any{"currency"},
		"aggregates": []any{map[string]any{"fn": "sum", "field": "amount_minor", "as": "amount"}},
		"filters":    map[string]any{"days_since_activity_at_least": 30},
	}
	toolReport := map[string]any{
		"report": "deals-by-stage", "group_by": report["group_by"], "aggregates": report["aggregates"],
		"filters": report["filters"],
	}
	got := map[string]json.RawMessage{
		"analytics route": d.rest(t, "POST", "/v1/analytics/query", query),
		"analytics tool":  d.tool(t, "run_analytics_query", query),
		"report route":    d.rest(t, "POST", "/v1/reports/deals-by-stage", report),
		"report tool":     d.tool(t, "run_report", toolReport),
	}
	for door, raw := range got {
		var answer struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal(raw, &answer); err != nil || len(answer.Rows) != 1 {
			t.Errorf("%s answered %s (%v), want one row", door, raw, err)
			continue
		}
		if sum := fmt.Sprint(answer.Rows[0]["amount"]); sum != fmt.Sprint(want) {
			t.Errorf("%s: deals quiet 30 days sum to %s, want %d: the deal touched 5 days ago must stay out", door, sum, want)
		}
	}
}

// daysAt judges the deal's quiet days at an instant, in the zone the
// installation holds.
func daysAt(t *testing.T, q *quietEnv, expr func(asOf string) string, deal ids.UUID, asOf time.Time) float64 {
	t.Helper()
	ctx := q.As(q.AdminUser, nil, AdminPerms)
	var days float64
	err := database.WithTx(ctx, q.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT "+expr("$1::timestamptz")+" FROM deal t WHERE t.id = $2", asOf, deal).Scan(&days)
	})
	if err != nil {
		t.Fatalf("counting quiet days: %v", err)
	}
	return days
}

func (q *quietEnv) setZone(t *testing.T, zone string) {
	t.Helper()
	q.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone)
}

// fileAt files a note on a deal at a stored instant, a whole day when dated.
func (q *quietEnv) fileAt(t *testing.T, column string, record ids.UUID, at time.Time, precision string) {
	t.Helper()
	activity := ids.NewV7()
	q.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, occurred_precision, source, captured_by)
		VALUES ($1, 'note', 'Filed', $2, $3, 'manual', 'human:x')`, activity, at, precision)
	q.WsExec(t, `INSERT INTO activity_link (id, activity_id, entity_type, `+column+`) VALUES ($1, $2, $3, $4)`,
		ids.NewV7(), activity, column[:len(column)-3], record)
}

// Days are the installation's calendar days, as the deal badge counts them.
// Take an activity at 03:00 on the 11th in Ho Chi Minh City. It is zero days
// old at 23:30 that day and one day old half an hour after midnight. Under a
// day has passed between the two.
func TestQuietDaysAreInstallationCalendarDays(t *testing.T) {
	q := setupQuiet(t)
	q.setZone(t, "Asia/Ho_Chi_Minh")
	deal, _ := q.deal(t, "Instant", q.Rep1, nil)
	q.fileAt(t, "deal_id", deal, time.Date(2026, 3, 10, 20, 0, 0, 0, time.UTC), "instant")
	for asOf, want := range map[time.Time]float64{
		time.Date(2026, 3, 11, 16, 30, 0, 0, time.UTC): 0,
		time.Date(2026, 3, 11, 17, 30, 0, 0, time.UTC): 1,
	} {
		if got := daysAt(t, q, quietfilters.DealDaysSinceActivityAt, deal, asOf); got != want {
			t.Errorf("at %s the deal is %v days quiet, want %v calendar days", asOf, got, want)
		}
	}
}

// A date-only activity counts from the date it names, whatever zone the
// installation holds, so two readers never disagree about a day entry.
func TestADateOnlyActivityCountsFromItsOwnDate(t *testing.T) {
	q := setupQuiet(t)
	q.setZone(t, "Pacific/Kiritimati")
	deal, _ := q.deal(t, "Dated", q.Rep1, nil)
	q.fileAt(t, "deal_id", deal, time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), "day")
	asOf := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
	if got := daysAt(t, q, quietfilters.DealDaysSinceActivityAt, deal, asOf); got != 2 {
		t.Errorf("a day entry for the 10th is %v days quiet on the 12th, want 2", got)
	}
}

func TestACompanysQuietDaysFollowTheSameRule(t *testing.T) {
	q := setupQuiet(t)
	q.setZone(t, "Asia/Ho_Chi_Minh")
	company := q.SeedCompany(t, "Rule Co", &q.Rep1)
	deal, _ := q.deal(t, "At company", q.Rep1, &company)
	q.fileAt(t, "company_id", company, time.Date(2026, 3, 10, 20, 0, 0, 0, time.UTC), "instant")
	asOf := time.Date(2026, 3, 11, 17, 30, 0, 0, time.UTC)
	if got := daysAt(t, q, quietfilters.CompanyDaysSinceActivityAt, deal, asOf); got != 1 {
		t.Errorf("the company is %v days quiet at %s, want 1 calendar day", got, asOf)
	}
}
