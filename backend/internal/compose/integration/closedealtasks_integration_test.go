// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Closing a deal with Complete all, as the confirm dialog does it over the
// wire. It reads the deal's open tasks, advances to lost, then bulk-completes
// the tasks it read. A task filed on the deal's company stays open.

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type closingTaskDTO struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	IsDone  bool   `json:"is_done"`
}

func fileClosingTask(t *testing.T, e *apptest.AppEnv, subject, entityType, entityID string) closingTaskDTO {
	t.Helper()
	var task closingTaskDTO
	if status := e.Call(t, "POST", "/v1/tasks", AnyMap{
		"subject": subject,
		"links":   []AnyMap{{"entity_type": entityType, "entity_id": entityID}},
	}, nil, &task); status != http.StatusCreated {
		t.Fatalf("filing the task %q → %d", subject, status)
	}
	return task
}

// dealOpenTasks is the dialog's read: the deal's own tasks, open ones only.
func dealOpenTasks(t *testing.T, e *apptest.AppEnv, dealID string) []closingTaskDTO {
	t.Helper()
	var page struct {
		Data []closingTaskDTO `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/activities?kind=task&limit=200&entity_type=deal&entity_id="+dealID,
		nil, nil, &page); status != http.StatusOK {
		t.Fatalf("reading the deal's tasks → %d", status)
	}
	var open []closingTaskDTO
	for _, task := range page.Data {
		if !task.IsDone {
			open = append(open, task)
		}
	}
	return open
}

func taskIsDone(t *testing.T, e *apptest.AppEnv, id string) bool {
	t.Helper()
	var done bool
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT is_done FROM activity WHERE id = $1`, id).Scan(&done)
	}); err != nil {
		t.Fatalf("reading task %s: %v", id, err)
	}
	return done
}

func batchAuditRows(t *testing.T, e *apptest.AppEnv, batchID, taskID string) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'activity' AND entity_id = $2`,
			batchID, taskID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the audit rows of task %s: %v", taskID, err)
	}
	return n
}

func TestClosingADealWithCompleteAllCompletesOnlyTheDealsOwnTasks(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)
	companyID := readDealCompany(t, e, dealID)

	onDeal := []closingTaskDTO{
		fileClosingTask(t, e, "Send the final invoice", "deal", dealID),
		fileClosingTask(t, e, "Return the loan device", "deal", dealID),
	}
	onCompany := fileClosingTask(t, e, "Plan the next account review", "company", companyID)

	listed := dealOpenTasks(t, e, dealID)
	if len(listed) != len(onDeal) {
		t.Fatalf("the dialog's read lists %d tasks, want the deal's own %d and not the company's: %+v",
			len(listed), len(onDeal), listed)
	}
	for _, task := range listed {
		if task.ID == onCompany.ID {
			t.Fatal("the dialog's read lists a task filed on the deal's company")
		}
	}

	if status := e.Call(t, "POST", "/v1/deals/"+dealID+"/advance",
		AnyMap{"to_stage_id": stages.Lost, "lost_reason": "went with a competitor", "lost_reason_code": "competitor"}, nil, nil); status != http.StatusOK {
		t.Fatalf("losing the deal → %d", status)
	}

	items := make([]AnyMap, 0, len(listed))
	for _, task := range listed {
		items = append(items, AnyMap{"id": task.ID, "version": task.Version})
	}
	change := AnyMap{"record_type": "worklist_item", "verb": "complete", "items": items}
	var preview bulkPreviewDTO
	if status := e.Call(t, "POST", "/v1/bulk/preview", change, nil, &preview); status != http.StatusOK || preview.Count != len(listed) {
		t.Fatalf("previewing Complete all → %d %+v, want every listed task", status, preview)
	}
	change["confirm_token"] = preview.ConfirmToken
	var result bulkResultDTO
	if status := e.Call(t, "POST", "/v1/bulk/execute", change,
		map[string]string{"Idempotency-Key": "close-deal-complete-all"}, &result); status != http.StatusOK {
		t.Fatalf("Complete all → %d", status)
	}
	if result.Changed != len(listed) || len(result.Skipped) != 0 {
		t.Fatalf("Complete all → %+v, want every listed task completed", result)
	}

	for _, task := range onDeal {
		if !taskIsDone(t, e, task.ID) {
			t.Errorf("deal task %s is still open after Complete all", task.ID)
		}
		if n := batchAuditRows(t, e, result.BatchID, task.ID); n != 1 {
			t.Errorf("deal task %s has %d audit rows in the batch, want 1", task.ID, n)
		}
	}
	if taskIsDone(t, e, onCompany.ID) {
		t.Error("Complete all completed a task filed on the deal's company")
	}
	if n := batchAuditRows(t, e, result.BatchID, onCompany.ID); n != 0 {
		t.Errorf("the company task has %d audit rows in the batch, want none", n)
	}
}

func readDealCompany(t *testing.T, e *apptest.AppEnv, dealID string) string {
	t.Helper()
	var deal struct {
		CompanyID string `json:"company_id"`
	}
	if status := e.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &deal); status != http.StatusOK || deal.CompanyID == "" {
		t.Fatalf("reading the deal's company → %d %+v", status, deal)
	}
	return deal.CompanyID
}
