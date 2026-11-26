// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Done pressed on a Home or Worklist row the reader's cache still serves, after
// a worker moved the task's version underneath it.

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// homeTaskVersion answers the version Home lists the task at, and whether it
// lists the task at all.
func homeTaskVersion(t *testing.T, e *apptest.AppEnv, task ids.UUID) (int64, bool) {
	t.Helper()
	var page struct {
		Queue []struct {
			ID      ids.UUID `json:"id"`
			Version *int64   `json:"version"`
		} `json:"queue"`
	}
	if status := e.Call(t, "GET", homeWorklistPath, nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET worklist → %d", status)
	}
	for _, item := range page.Queue {
		if item.ID == task {
			if item.Version == nil {
				t.Fatal("Home lists the task without the version its Done sends")
			}
			return *item.Version, true
		}
	}
	return 0, false
}

// cachedTaskMovedByAWorker lists an overdue task on the reader's cached Home,
// then lets workerWrite change it outside the reader's session. It answers the
// task and the version the cache still serves.
func cachedTaskMovedByAWorker(t *testing.T, e *apptest.AppEnv, email, workerWrite string) (ids.UUID, int64) {
	t.Helper()
	ctx := context.Background()
	var admin, task ids.UUID
	if err := e.Owner.QueryRow(ctx, `SELECT id FROM app_user WHERE email = $1`, email).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, due_at, is_done, assignee_id, source, captured_by)
		VALUES ('task', 'Chase the signed order', now() - interval '1 hour', false, $1, 'manual', $2)
		RETURNING id`, admin, "human:"+admin.String()).Scan(&task); err != nil {
		t.Fatal(err)
	}
	read, listed := homeTaskVersion(t, e, task)
	if !listed {
		t.Fatal("Home does not list the overdue task")
	}
	if _, err := e.Owner.Exec(ctx, workerWrite, task); err != nil {
		t.Fatal(err)
	}
	if cached, _ := homeTaskVersion(t, e, task); cached != read {
		t.Fatalf("Home served version %d after the worker write, want the cached %d", cached, read)
	}
	return task, read
}

func TestDoneOnACachedTaskAWorkerCompletedSucceedsAndLeavesHome(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Done", "admin@homedone.test", "Admin")
	task, read := cachedTaskMovedByAWorker(t, e, "admin@homedone.test",
		`UPDATE activity SET is_done = true, done_at = now() WHERE id = $1`)

	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "PATCH", "/v1/activities/"+task.String(), AnyMap{"is_done": true},
		map[string]string{"If-Match": strconv.FormatInt(read, 10)}, &problem); status != http.StatusOK {
		t.Fatalf("Done on a task already done → %d %q, want 200", status, problem.Code)
	}
	if _, listed := homeTaskVersion(t, e, task); listed {
		t.Error("Home still lists the task after Done succeeded")
	}
}

func TestAStaleDoneOnARenamedTaskRefusesAndHomeThenServesTheNewVersion(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Home Skew", "admin@homeskew.test", "Admin")
	task, read := cachedTaskMovedByAWorker(t, e, "admin@homeskew.test",
		`UPDATE activity SET subject = 'Chase the countersigned order' WHERE id = $1`)

	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "PATCH", "/v1/activities/"+task.String(), AnyMap{"is_done": true},
		map[string]string{"If-Match": strconv.FormatInt(read, 10)}, &problem); status != http.StatusConflict || problem.Code != "version_skew" {
		t.Fatalf("a stale Done on a renamed task → %d %q, want 409 version_skew", status, problem.Code)
	}
	fresh, listed := homeTaskVersion(t, e, task)
	if !listed || fresh == read {
		t.Fatalf("after the skew Home lists the task %v at version %d, want the moved version, not %d", listed, fresh, read)
	}
	if status := e.Call(t, "PATCH", "/v1/activities/"+task.String(), AnyMap{"is_done": true},
		map[string]string{"If-Match": strconv.FormatInt(fresh, 10)}, nil); status != http.StatusOK {
		t.Errorf("Done at the refetched version → %d, want 200", status)
	}
}
