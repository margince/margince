// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// listSavedQuestions pages GET /analytics/runs as reader, two at a time,
// and returns every id it was given.
func listSavedQuestions(reader context.Context, t *testing.T, e *forecastEnv) []string {
	t.Helper()
	h := analyticsQueryHandlers{db: InstallationDB(e.Pool)}
	limit := 2
	var cursor *string
	seen := []string{}
	for range 10 {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/analytics/runs", nil).WithContext(reader)
		h.ListReportRuns(response, request, crmcontracts.ListReportRunsParams{Cursor: cursor, Limit: &limit})
		if response.Code != http.StatusOK {
			t.Fatalf("list: status %d: %s", response.Code, response.Body.String())
		}
		var page crmcontracts.ReportRunList
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, run := range page.Data {
			seen = append(seen, run.Id.String())
		}
		if !page.Page.HasMore {
			return seen
		}
		cursor = page.Page.NextCursor
	}
	t.Fatal("the list never ended")
	return nil
}

// saveQuestions saves count questions as asker in one transaction, so they
// share one created_at.
func saveQuestions(asker context.Context, t *testing.T, e *forecastEnv, count int) []string {
	t.Helper()
	q := analyticsquery.Query{Entity: "open-deals-per-company", Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}}}
	saved := []string{}
	if err := InstallationDB(e.Pool).Tx(asker, func(tx pgx.Tx) error {
		for range count {
			answer, err := RunAnalyticsQuery(asker, tx, q, analyticsquery.DefaultFloor)
			if err != nil {
				return err
			}
			id, err := SaveReportRun(asker, tx, q, answer, analyticsquery.DefaultFloor)
			if err != nil {
				return err
			}
			saved = append(saved, id.String())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return saved
}

// A reader lists only the questions they saved, each once, across pages that
// break between two saved at the same instant.
func TestASavedQuestionIsListedForItsAskerOnly(t *testing.T) {
	e := setupForecast(t)
	mine := e.ownLensRepCtx(e.Rep1)
	theirs := e.ownLensRepCtx(e.Rep3)
	saved := saveQuestions(mine, t, e, 3)
	other := saveQuestions(theirs, t, e, 1)

	listed := listSavedQuestions(mine, t, e)
	if len(listed) != 3 {
		t.Fatalf("Rep1's list has %d entries %v, want their 3 saved questions once each", len(listed), listed)
	}
	for _, id := range saved {
		found := 0
		for _, got := range listed {
			if got == id {
				found++
			}
		}
		if found != 1 {
			t.Errorf("question %s is listed %d times, want once", id, found)
		}
	}
	if got := listSavedQuestions(theirs, t, e); len(got) != 1 || got[0] != other[0] {
		t.Fatalf("Rep3's list is %v, want only their own %v", got, other)
	}
}
