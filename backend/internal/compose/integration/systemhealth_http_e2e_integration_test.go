// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func enqueueHealthFixtures(t *testing.T, e *apptest.AppEnv, n int) {
	t.Helper()
	inserter := insertOnlyRunner(t)
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		for range n {
			if err := inserter.EnqueueTx(t.Context(), tx, compose.CaptureClassifyArgs{}, &river.InsertOpts{ScheduledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystemHealthFindsRunnableWorkWithoutARecentFailure(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 3)
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	found := false
	for _, f := range report.Findings {
		if f.Code == "queue_overdue" {
			found = true
			if f.Count == nil || *f.Count != 3 || !f.RequiresAction || f.JobKinds == nil || len(*f.JobKinds) != 1 || (*f.JobKinds)[0] != (compose.CaptureClassifyArgs{}).Kind() {
				t.Fatalf("overdue count: %+v", f)
			}
		}
	}
	if !found || report.Assessment != "attention" {
		t.Fatalf("overdue work absent: %+v", report)
	}
}

func TestJobHistoryPagesCompleteGroupsAndDisclosesRetentionChanges(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 65)
	// River's terminal transition is the database boundary; insertion uses the production writer.
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET state='discarded',finalized_at=now() WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	var first crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history", nil, nil, &first); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(first.Runs) != 50 || !first.HasMore || first.NextCursor == nil {
		t.Fatalf("first page: %+v", first)
	}
	if len(first.Groups) != 1 || first.Groups[0].Count != 65 {
		t.Fatalf("groups are not complete: %+v", first.Groups)
	}
	path := "/v1/admin/job-health/history?cursor=" + url.QueryEscape(*first.NextCursor)
	var second crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", path, nil, nil, &second); status != http.StatusOK {
		t.Fatalf("continuation status %d", status)
	}
	if len(second.Runs) != 15 || second.HasMore || !second.PopulationMayHaveChanged {
		t.Fatalf("second page: %+v", second)
	}
	seen := map[int64]bool{}
	for _, row := range first.Runs {
		if row.JobId == nil {
			t.Fatal("missing ID")
		}
		seen[*row.JobId] = true
	}
	for _, row := range second.Runs {
		if row.JobId == nil || seen[*row.JobId] {
			t.Fatal("duplicate or missing run ID")
		}
		seen[*row.JobId] = true
	}
	var before crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &before); status != http.StatusOK {
		t.Fatalf("summary status %d", status)
	}
	found := false
	for _, source := range before.Sources {
		if source.Id == "jobs" {
			found = true
			if source.Condition != "unverified" || source.Availability != "available" {
				t.Fatalf("retained fault was not observable: %+v", source)
			}
		}
	}
	if !found {
		t.Fatal("jobs source missing")
	}
	if _, err := e.Owner.Exec(t.Context(), `DELETE FROM river_job WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	var summary crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &summary); status != http.StatusOK {
		t.Fatalf("summary status %d", status)
	}
	for _, source := range summary.Sources {
		if source.Id == "jobs" && (source.Condition == "recovered" || source.Availability != "unknown") {
			t.Fatalf("expired history proved recovery: %+v", source)
		}
	}
}

func TestHistoryCursorCannotChangeItsFilters(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 51)
	if _, err := e.Owner.Exec(context.Background(), `UPDATE river_job SET state='discarded',finalized_at=now() WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	var first crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history", nil, nil, &first); status != http.StatusOK || first.NextCursor == nil {
		t.Fatalf("history: %d %+v", status, first)
	}
	path := "/v1/admin/job-health/history?days=1&cursor=" + url.QueryEscape(*first.NextCursor)
	if status := e.Call(t, "GET", path, nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("changed-filter status %d", status)
	}
}

func TestHealthQueueAssessmentAndDetailUseTheSameOwner(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 1)
	var detail crmcontracts.JobHealth
	if status := e.Call(t, "GET", "/v1/admin/job-health", nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, kind := range detail.Kinds {
		if kind.Kind == (compose.CaptureClassifyArgs{}).Kind() {
			spec, ok := jobs.SpecFor(kind.Kind)
			if !ok || kind.ConditionCode == nil || *kind.ConditionCode != "queue_overdue" || kind.ExpectationSeconds == nil || *kind.ExpectationSeconds != int(spec.Cadence.Fixed.Seconds()) {
				t.Fatalf("detail lost the assessment: %+v", kind)
			}
			return
		}
	}
	t.Fatal("queue omitted")
}

func TestSystemHealthReadsKeepDomainEvidenceUnchanged(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 3)
	counts := func() [3]int64 {
		var n [3]int64
		if err := e.Owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM audit_log),
 (SELECT count(*) FROM event_outbox),(SELECT count(*) FROM river_job)`).Scan(&n[0], &n[1], &n[2]); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := counts()
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary status %d", status)
	}
	seen := map[string]bool{}
	for _, source := range report.Sources {
		if source.Availability == "withheld" || seen[source.Id] {
			t.Fatalf("Admin projection lost a source: %+v", source)
		}
		seen[source.Id] = true
	}
	for range 3 {
		if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, nil); status != http.StatusOK {
			t.Fatalf("summary status %d", status)
		}
	}
	if after := counts(); after != before {
		t.Fatalf("health read changed evidence: %v -> %v", before, after)
	}
}

func TestSystemHealthProjectsRevokedSourcesAndKeepsSearchReadable(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var role roleWire
	if status := e.Call(t, "POST", "/v1/roles", map[string]string{"copy_from": "admin", "name": "Health reader"}, nil, &role); status != http.StatusCreated {
		t.Fatalf("custom role: %d", status)
	}
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(context.Background(), `DELETE FROM field_mask WHERE role_key=$1`, role.Key); err != nil {
			t.Error(err)
		}
	})

	for _, object := range []string{"job_health", "data_task", "ai_diagnostics", "installation_settings"} {
		body := map[string]bool{"create": false, "read": false, "update": false, "delete": false}
		if status := e.Call(t, "PATCH", "/v1/roles/"+role.Key+"/objects/"+object, body, nil, nil); status != http.StatusOK {
			t.Fatalf("revoke %s: %d", object, status)
		}
	}
	assignSessionRole(t, e, role.Key)
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	for _, source := range report.Sources {
		if source.Id == "search" {
			if source.Availability == "withheld" {
				t.Fatal("search grant lost")
			}
		} else if source.Availability != "withheld" || source.ObservedAt != nil || source.Count != nil {
			t.Fatalf("revoked source leaked evidence: %+v", source)
		}
	}
	if len(report.Findings) != 0 {
		t.Fatalf("revoked findings remain: %+v", report.Findings)
	}
	if status := e.Call(t, "GET", "/v1/admin/job-health/history", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("history after revocation: %d", status)
	}
}

func TestSystemHealthDoesNotTreatAMatchingSearchModelAsCompleteIndexEvidence(t *testing.T) {
	router := embedReindexRouter(t, "health-marker-model")
	e := setupEmbedReindex(t, router)
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	for _, source := range report.Sources {
		if source.Id == "search" {
			if source.Availability != "unknown" || source.Condition != "unknown" || source.Code != "search_unverified" {
				t.Fatalf("marker overstated coverage: %+v", source)
			}
			return
		}
	}
	t.Fatal("search source missing")
}

func TestHistoryKeepsCurrentRetriesBeyondTheFinishedRunWindow(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 2)
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET created_at=now()-interval '30 days',attempted_at=now()-interval '30 days',
 state=CASE WHEN id%2=0 THEN 'retryable'::river_job_state ELSE 'discarded'::river_job_state END,
 finalized_at=CASE WHEN id%2=0 THEN NULL ELSE now()-interval '30 days' END WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	var report crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history?days=1", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("history: %d", status)
	}
	if len(report.Runs) != 1 || report.Runs[0].State != "retryable" || len(report.Groups) != 1 || report.Groups[0].Count != 1 {
		t.Fatalf("current retry hidden or old finished run included: %+v", report)
	}
}

func TestMissingRestoreEvidenceLeavesHealthIncomplete(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	if report.Assessment != "incomplete" || report.Complete {
		t.Fatalf("missing evidence was overstated: %+v", report)
	}
	for _, finding := range report.Findings {
		if finding.Source == "recovery" {
			t.Fatalf("no drill was treated as a known fault: %+v", finding)
		}
	}
}

func TestHistoryExcludesAnotherWorkspacesRetainedRuns(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 1)
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET state='discarded',finalized_at=now() WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	seedRiverRow(t, e, riverRow{kind: (compose.CaptureClassifyArgs{}).Kind(), workspace: ids.NewV7().String(), state: "discarded", attempt: 1})
	var report crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("history: %d", status)
	}
	if len(report.Runs) != 1 || len(report.Groups) != 1 || report.Groups[0].Count != 1 || !report.Groups[0].FleetWide {
		t.Fatalf("history crossed its scope: %+v", report)
	}
}

func TestExpectedQueueWorkAndCancellationDoNotBecomeProblems(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	inserter := insertOnlyRunner(t)
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return inserter.EnqueueTx(t.Context(), tx, compose.CaptureClassifyArgs{}, &river.InsertOpts{ScheduledAt: clocktest.Now(t).Add(time.Hour)})
	}); err != nil {
		t.Fatal(err)
	}
	var report crmcontracts.SystemHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	processing := false
	for _, source := range report.Sources {
		if source.Id == "jobs" {
			processing = source.Condition == "processing" && source.Availability == "unknown"
		}
	}
	if !processing {
		t.Fatalf("current work or missing completion coverage lost: %+v", report.Sources)
	}
	for _, finding := range report.Findings {
		if finding.Source == "jobs" {
			t.Fatalf("scheduled work needs no intervention: %+v", finding)
		}
	}
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET state='cancelled',finalized_at=now() WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("summary: %d", status)
	}
	for _, finding := range report.Findings {
		if finding.Source == "jobs" {
			t.Fatalf("intentional cancellation needs no intervention: %+v", finding)
		}
	}
	var history crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history?state=cancelled", nil, nil, &history); status != http.StatusOK || len(history.Runs) != 1 {
		t.Fatalf("cancelled run not inspectable: %d %+v", status, history)
	}
}

func TestHistoryContinuationExcludesRunsFinishedAfterItsCutoff(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	enqueueHealthFixtures(t, e, 51)
	kind := (compose.CaptureClassifyArgs{}).Kind()
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET state='discarded',finalized_at=now() WHERE kind=$1`, kind); err != nil {
		t.Fatal(err)
	}
	var first crmcontracts.JobHealthHistory
	if status := e.Call(t, "GET", "/v1/admin/job-health/history", nil, nil, &first); status != http.StatusOK || first.NextCursor == nil {
		t.Fatalf("first history page: %d %+v", status, first)
	}
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET finalized_at=$1 WHERE id=(SELECT min(id) FROM river_job WHERE kind=$2)`, first.Cutoff.Add(time.Second), kind); err != nil {
		t.Fatal(err)
	}
	var next crmcontracts.JobHealthHistory
	path := "/v1/admin/job-health/history?cursor=" + url.QueryEscape(*first.NextCursor)
	if status := e.Call(t, "GET", path, nil, nil, &next); status != http.StatusOK {
		t.Fatalf("continuation: %d", status)
	}
	if len(next.Runs) != 0 || next.HasMore || len(next.Groups) != 1 || next.Groups[0].Count != 50 || !next.PopulationMayHaveChanged || !next.Cutoff.Equal(first.Cutoff) {
		t.Fatalf("finished work crossed the reporting cutoff: %+v", next)
	}
}
