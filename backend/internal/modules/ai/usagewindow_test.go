// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The report and the summary refuse a usage window by one rule, because both
// aggregate every metered call in it.
func TestAUsageWindowIsInOrderAndAtMostAYear(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name, code, field string
		to                time.Time
	}{
		{"a month", "", "", from.AddDate(0, 1, 0)},
		{"exactly the cap", "", "", from.Add(maxUsageWindowDays * 24 * time.Hour)},
		{"a day past the cap", "window_too_wide", "to", from.Add((maxUsageWindowDays + 1) * 24 * time.Hour)},
		{"before it starts", "window_inverted", "from", from.AddDate(0, 0, -1)},
	} {
		refusal := usageWindowRefusal(from, c.to)
		code, field := "", ""
		if refusal != nil {
			code = refusal.Code
			if len(refusal.Fields) == 1 {
				field = refusal.Fields[0].Field
			}
		}
		if code != c.code || field != c.field {
			t.Errorf("%s: refusal %q on %q, want %q on %q", c.name, code, field, c.code, c.field)
		}
	}
}

// Both doors take the rule, and refuse before they read a single call: the
// meter here has no database, so reaching it would panic.
func TestBothUsageReadsRefuseAWindowBeforeReadingIt(t *testing.T) {
	h := Handlers{meter: NewMeter(nil)}
	from := openapi_types.Date{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	tooLate := openapi_types.Date{Time: from.AddDate(2, 0, 0)}
	tooEarly := openapi_types.Date{Time: from.AddDate(0, 0, -1)}
	for _, c := range []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
	}{
		{"the report, too wide", func(w http.ResponseWriter, r *http.Request) {
			h.GetAiUsage(w, r, crmcontracts.GetAiUsageParams{From: &from, To: &tooLate})
		}},
		{"the summary, too wide", func(w http.ResponseWriter, r *http.Request) {
			h.GetAiUsageSummary(w, r, crmcontracts.GetAiUsageSummaryParams{From: &from, To: &tooLate})
		}},
		{"the summary, inverted", func(w http.ResponseWriter, r *http.Request) {
			h.GetAiUsageSummary(w, r, crmcontracts.GetAiUsageSummaryParams{From: &from, To: &tooEarly})
		}},
	} {
		rec := httptest.NewRecorder()
		c.call(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/usage", nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, want 422", c.name, rec.Code)
		}
	}
}
