// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every `date` column states whose calendar it is a day in.
//
// A `date` carries no zone, so the schema alone cannot say whether 2026-12-01
// began at midnight in the installation's zone or at midnight UTC — and the two
// disagree for hours every day, which is when a deal reads as overdue on its own
// close date or a rate takes effect a day early. The product's rule is that an
// as-of day is read in the installation's zone; this registry is where each
// column says it follows that rule, or says why it does not.
//
// The subject is DERIVED from the head catalog, so a new `date` column fails
// here until it has an entry, and an entry for a column that has gone fails too:
// the registry and the schema are held equal in both directions.

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// dayCalendar names the calendar a stored day was cut in.
type dayCalendar string

const (
	// installationDay is the rule: a day in the installation's configured zone,
	// derived through storekit.WorkspaceDay, or a date a user picked that is
	// judged against the installation's today.
	installationDay dayCalendar = "installation zone"
	// utcDay is a day cut at UTC midnight. Every one carries a reason.
	utcDay dayCalendar = "UTC"
	// suppliedDay is a date taken as an outside system wrote it, in a zone the
	// product never learns, and compared with no today.
	suppliedDay dayCalendar = "as supplied"
)

// dateColumnCalendar is one column's statement. why is required for every
// calendar but installationDay, because an exception nobody explained is the
// convention this registry replaces. drift names, with its issue, a reader that
// asks with a different today than the writer used.
type dateColumnCalendar struct {
	calendar dayCalendar
	why      string
	drift    string
}

// readersDisagree is the issue that tracks every drift note below; a column's
// note goes when its readers agree with its writer.
const readersDisagree = "#6714"

var dateColumnCalendars = map[string]dateColumnCalendar{
	"ai_model_rate.effective_date": {
		calendar: utcDay,
		why:      "the AI price sheet's effective day, guarded and matched against todayUTC() on both sides",
		drift:    "belongs in the installation zone once RateStore can resolve it, like fx_rate beside it; " + readersDisagree,
	},
	"ai_usage.day": {
		calendar: utcDay,
		why:      "a metering counter: written and windowed on the UTC calendar, the monthly AI budget included, so every reader of one day's spend sums the same rows",
	},
	"brief_item.returned_after_dismissal_on": {calendar: installationDay},
	"brief_run.local_day":                    {calendar: installationDay},
	"capture_auto_enrich_budget.budget_date": {
		calendar: utcDay,
		why:      "one shared daily enrichment cap, keyed explicitly on (now() AT TIME ZONE 'UTC')::date so the cap resets at one instant everywhere",
	},
	"capture_backfill.after_date": {
		calendar: utcDay,
		why:      "the provider window's lower bound, handed to Graph and Gmail as a UTC instant; it is a fetch boundary, never a business day",
	},
	"capture_digest.digest_date": {
		calendar: utcDay,
		why:      "the capture digest is built and labelled on the UTC calendar",
		drift:    "it sits beside brief_run.local_day, an installation day; " + readersDisagree,
	},
	"close_date_run.as_of":                       {calendar: installationDay},
	"contract.cancellation_effective_on":         {calendar: installationDay},
	"contract.cancellation_notice_on":            {calendar: installationDay},
	"contract.ends_on":                           {calendar: installationDay},
	"contract.fx_rate_date":                      {calendar: installationDay},
	"contract.renewal_on":                        {calendar: installationDay, drift: "company360 reads it through ::timestamptz, a session-zone midnight; " + readersDisagree},
	"contract.signed_on":                         {calendar: installationDay},
	"contract.starts_on":                         {calendar: installationDay},
	"deal.expected_close_date":                   {calendar: installationDay, drift: "the assurance close_past rule and the days_ago list filter judge it against a UTC today; " + readersDisagree},
	"deal.fx_rate_date":                          {calendar: installationDay},
	"deal.wait_until":                            {calendar: installationDay, drift: "the stall formulas end the pause at 00:00 UTC; " + readersDisagree},
	"deal_forecast_history.close_date_at_change": {calendar: installationDay},
	"deal_risk_day.local_day":                    {calendar: installationDay},
	"deal_stage_history.fx_date_at_change": {
		calendar: installationDay,
		drift:    "stage valuation picks its rate with the process clock's day, not the installation's; " + readersDisagree,
	},
	"deal_suggestion.proposed_close_date": {calendar: installationDay},
	"extension_ingest_refusal.day": {
		calendar: utcDay,
		why:      "an operator health counter, written and windowed with time.Now().UTC() on both sides",
	},
	"finance_invoice.due_at": {
		calendar: utcDay,
		why:      "the finance provider's date, which the offline ledger anchors at a fixed UTC origin",
		drift:    "compared as an instant, so an invoice reads overdue from 00:00 UTC on its own due day; " + readersDisagree,
	},
	"finance_invoice.fx_rate_date": {
		calendar: utcDay,
		why:      "a copy of issued_at for a base-currency invoice, in the same UTC calendar",
	},
	"finance_invoice.issued_at": {
		calendar: utcDay,
		why:      "the finance provider's date, which the offline ledger anchors at a fixed UTC origin",
	},
	"forecast_call.period_end":                   {calendar: installationDay},
	"forecast_call.period_start":                 {calendar: installationDay},
	"forecast_contribution.effective_close_date": {calendar: installationDay},
	"forecast_contribution.fx_date":              {calendar: installationDay},
	"forecast_snapshot.local_day":                {calendar: installationDay},
	"forecast_snapshot.period_end":               {calendar: installationDay},
	"forecast_snapshot.period_start":             {calendar: installationDay},
	"fx_rate.rate_date":                          {calendar: installationDay, drift: "the open-pipeline rollup, company 360, the brief and the deal list's base value ask it with a UTC or session-zone today; " + readersDisagree},
	"linkedin_connection.connected_on":           {calendar: suppliedDay, why: "parsed from the member's LinkedIn export as written; only a dedupe key and a subject-access export read it"},
	"notification_digest_run.digest_date":        {calendar: installationDay},
	"offer.fx_rate_date":                         {calendar: installationDay},
	"offer.valid_until":                          {calendar: installationDay},
	"partner.joined_at":                          {calendar: installationDay},
	"partner.next_step_due_at":                   {calendar: installationDay, drift: "the days_ago list filter judges it against a UTC today; " + readersDisagree},
	"partner.renews_at":                          {calendar: installationDay},
	"project.ended_at":                           {calendar: installationDay},
	"project.started_at":                         {calendar: installationDay},
	"project.target_end_date":                    {calendar: installationDay, drift: "the days_ago list filter judges it against a UTC today; " + readersDisagree},
	"relationship.ended_at":                      {calendar: installationDay, drift: "the UI writes the browser's day and the employment reads compare it with CURRENT_DATE; " + readersDisagree},
	"relationship.started_at":                    {calendar: installationDay},
	"team_weekly_review.local_week_start":        {calendar: installationDay},
	"team_weekly_review_outlook.period_end":      {calendar: installationDay},
	"team_weekly_review_outlook.period_start":    {calendar: installationDay},
	"weekly_plan.local_week_start":               {calendar: installationDay},
	"weekly_plan_commitment.due_on":              {calendar: installationDay},
	"weekly_review.local_week_start":             {calendar: installationDay},
	"weekly_review_outlook.period_end":           {calendar: installationDay},
	"weekly_review_outlook.period_start":         {calendar: installationDay},
}

// schemaDateColumns derives every `date` column in the head catalog as
// "table.column", through the same line shape the undo gate reads.
func schemaDateColumns(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var columns []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := dateColumnLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			columns = append(columns, m[1]+"."+m[2])
		}
	}
	sort.Strings(columns)
	return columns
}

func TestEveryDateColumnStatesItsCalendar(t *testing.T) {
	t.Parallel()
	columns := schemaDateColumns(t)
	inSchema := make(map[string]bool, len(columns))
	for _, column := range columns {
		inSchema[column] = true
		if _, stated := dateColumnCalendars[column]; !stated {
			t.Errorf("%s is a `date` column with no stated calendar. Add it to dateColumnCalendars "+
				"in backend/gates/datecolumncalendars_test.go: installationDay when its day is "+
				"cut with storekit.WorkspaceDay or picked by a user and judged against the "+
				"installation's today, or another calendar with the reason it differs.", column)
		}
	}
	for column := range dateColumnCalendars {
		if !inSchema[column] {
			t.Errorf("dateColumnCalendars states a calendar for %s, which the head catalog does "+
				"not hold as a `date` column — remove the entry, or this registry describes a "+
				"schema that is gone", column)
		}
	}
}

func TestADateColumnOutsideTheInstallationZoneSaysWhy(t *testing.T) {
	t.Parallel()
	for column, stated := range dateColumnCalendars {
		switch stated.calendar {
		case installationDay:
		case utcDay, suppliedDay:
			if strings.TrimSpace(stated.why) == "" {
				t.Errorf("%s is a %s day with no reason; the installation zone is the rule, "+
					"so an exception says why it holds", column, stated.calendar)
			}
		default:
			t.Errorf("%s states calendar %q, which is none of the three this registry knows",
				column, stated.calendar)
		}
	}
}

func TestADriftNoteNamesTheIssueThatTracksIt(t *testing.T) {
	t.Parallel()
	for column, stated := range dateColumnCalendars {
		if stated.drift != "" && !strings.HasSuffix(stated.drift, readersDisagree) {
			t.Errorf("%s notes reader drift without citing %s, the issue that tracks every "+
				"drift note: %q", column, readersDisagree, stated.drift)
		}
	}
}

// Planted lines keep an edit to the shared catalog reader from narrowing what it
// sees: a column the reader misses is missing from the registry check too.
func TestTheDateColumnReaderSeesADateAndNothingElse(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]bool{
		"public.deal.expected_close_date date gen=- def=-":                         true,
		"public.fx_rate.rate_date date NOT NULL gen=- def=-":                       true,
		"public.deal.created_at timestamp with time zone NOT NULL gen=- def=now()": false,
		"public.deal.date_source text gen=- def=-":                                 false,
	} {
		if got := dateColumnLine.MatchString(line); got != want {
			t.Errorf("the date-column reader matched %q = %v, want %v", line, got, want)
		}
	}
}
