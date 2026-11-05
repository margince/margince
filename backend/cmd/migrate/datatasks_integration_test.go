// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// execute runs the corrections through the executor and never through the
// operation that queues work for a worker. Both are replaced, so the command's
// own wiring is what is under test.
func TestDataTasksExecuteRunsInProcessAndQueuesNothing(t *testing.T) {
	dsn := migratedDatabase(t, "_dtexec")
	savedOp, savedExec := dataTaskOperation, dataTaskExecutor
	t.Cleanup(func() { dataTaskOperation, dataTaskExecutor = savedOp, savedExec })
	dataTaskOperation = func(context.Context, *pgxpool.Pool, string) ([]datatask.Progress, error) {
		t.Error("execute reached the operation that queues work for the worker")
		return nil, errors.New("not here")
	}
	for name, c := range map[string]struct {
		run  compose.DataTaskRun
		fail bool
	}{
		"finished":      {compose.DataTaskRun{Progress: []datatask.Progress{{TaskID: "core/1_fix", Step: "a", Status: datatask.Done, Scanned: 2}}}, false},
		"left in scope": {compose.DataTaskRun{Progress: []datatask.Progress{{TaskID: "core/1_fix", Step: "a", Status: datatask.Done, Scanned: 2}}, LeftInScope: map[string]int64{"core/1_fix/a": 3}}, true},
		"awaiting review": {compose.DataTaskRun{Progress: []datatask.Progress{
			{TaskID: "core/2_manual", Step: "a", Status: datatask.Pending, BlockedReason: datatask.ReviewRequired},
		}}, true},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			dataTaskExecutor = func(context.Context, *pgxpool.Pool, int) (compose.DataTaskRun, error) {
				called = true
				return c.run, nil
			}
			var out bytes.Buffer
			err := runDataTasks(context.Background(), []string{"execute", "--dsn", dsn}, &out, io.Discard)
			if !called || (err != nil) != c.fail {
				t.Errorf("executor called %v, err %v; want a failure %v", called, err, c.fail)
			}
			var report struct {
				Verdict compose.DataTaskVerdict `json:"verdict"`
			}
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Errorf("execute printed no report: %v (%q)", err, out.String())
			}
		})
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	var jobs int
	if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM river_job`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Errorf("execute queued %d jobs", jobs)
	}
}
