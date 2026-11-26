// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An archived report keeps its snapshots readable, takes no new snapshot or
// schedule, and is listed apart from the live library.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAnArchivedReportIsListedApartAndTakesNoNewWork(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	selection := AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{"bookings_won"}}
	id := createdRecord(t, e, "/v1/analytics/reports", AnyMap{"name": "Archived review", "audience": "private", "selection": selection})
	path := "/v1/analytics/reports/" + id
	mustCall(t, e, "DELETE", path, nil, http.StatusOK, nil)

	var live, archived crmcontracts.ReportingReportList
	mustCall(t, e, "GET", "/v1/analytics/reports", nil, http.StatusOK, &live)
	mustCall(t, e, "GET", "/v1/analytics/reports?archived=true", nil, http.StatusOK, &archived)
	if len(live.Data) != 0 {
		t.Errorf("the live library still lists the archived report: %+v", live.Data)
	}
	if len(archived.Data) != 1 || archived.Data[0].Id.String() != id || archived.Data[0].ArchivedAt == nil {
		t.Fatalf("the archived library = %+v, want the one archived report", archived.Data)
	}

	var refused crmcontracts.Problem
	if status := e.Call(t, "POST", path+"/editions", nil, map[string]string{"Idempotency-Key": "after-archive"}, &refused); status != http.StatusConflict || refused.Code != "report_archived" {
		t.Errorf("a snapshot of an archived report → %d %q, want 409 report_archived", status, refused.Code)
	}
	schedule := AnyMap{"report_revision": 1, "frequency": "monthly", "day": 1, "local_time": "09:00", "enabled": false}
	if status := e.Call(t, "POST", path+"/schedules", schedule, nil, &refused); status != http.StatusConflict || refused.Code != "report_archived" {
		t.Errorf("a schedule of an archived report → %d %q, want 409 report_archived", status, refused.Code)
	}
}
