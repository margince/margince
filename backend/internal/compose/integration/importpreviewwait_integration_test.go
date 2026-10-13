// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// preview_import queues its test and waits a bounded time for the report. Past
// the wait it answers the run still testing, and read_import reaches the
// report. commit_import resumes a commit that failed.

import (
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// steppedClock hands each wait to the test, which fires it when it chooses.
type steppedClock struct{ asked chan chan time.Time }

func (c steppedClock) After(time.Duration) <-chan time.Time {
	tick := make(chan time.Time, 1)
	c.asked <- tick
	return tick
}

// waitedPreview is preview_import's answer with the copy a caller polls by.
type waitedPreview struct {
	Run struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	} `json:"run"`
	Report *struct {
		RowsRead int `json:"rows_read"`
	} `json:"report"`
	Next string `json:"next"`
}

func previewArgs(name string) map[string]any {
	return map[string]any{
		"object": "company", "csv": "name\n" + name + "\n", "mapping": map[string]string{"name": "display_name"},
	}
}

func TestPreviewWaitsThenAnswersTheReport(t *testing.T) {
	clock := steppedClock{asked: make(chan chan time.Time)}
	d := newTwoDoorsWith(t, "preview-waits",
		[]compose.Option{compose.WithImportPreviewWaitForTest(clock, 20*time.Second)}, "read", "write")
	answered := make(chan apptest.MCPResult, 1)
	go func() { answered <- d.mcp.Call(t, "preview_import", previewArgs("Waited GmbH")) }()

	var tick chan time.Time
	select {
	case tick = <-clock.asked:
	case early := <-answered:
		t.Fatalf("the preview answered without waiting: %s", early.Text)
	}
	if queued := importjobtest.Queued(t, d.e, compose.ImportValidateArgs{}.Kind()); len(queued) != 1 {
		t.Fatalf("%d test jobs queued while the preview waits, want its one", len(queued))
	}
	importjobtest.WorkQueued(t, d.e, compose.ImportValidateArgs{}.Kind())
	tick <- time.Time{}

	got := <-answered
	if got.IsError {
		t.Fatalf("the preview was refused: %s", got.Text)
	}
	var preview waitedPreview
	got.JSON(t, &preview)
	if preview.Run.State != "awaiting_approval" || preview.Next != "" {
		t.Fatalf("the preview answered %+v, want the report ready after one wait", preview)
	}
	if preview.Report == nil || preview.Report.RowsRead != 1 {
		t.Fatalf("the preview answered the report %+v, want the test's report of its one row", preview.Report)
	}
	select {
	case <-clock.asked:
		t.Fatal("the preview waited again after its report was ready")
	default:
	}
}

func TestPreviewAnswersValidatingPastTheWait(t *testing.T) {
	d := newTwoDoors(t, "preview-past-wait", "read", "write")
	var preview waitedPreview
	decode(t, d.tool(t, "preview_import", previewArgs("Unwaited GmbH")), &preview)
	if preview.Run.State != "validating" || !strings.Contains(preview.Next, "read_import") || preview.Report != nil {
		t.Fatalf("the preview answered %+v, want validating, no report and the copy to poll read_import", preview)
	}

	var report struct {
		State string `json:"state"`
	}
	readReport := map[string]any{"view": "report", "run_id": preview.Run.RunID}
	if decode(t, d.tool(t, "read_import", readReport), &report); report.State != "validating" {
		t.Fatalf("read_import answered %q, want the run still validating", report.State)
	}
	importjobtest.Drain(t, d.e)
	if decode(t, d.tool(t, "read_import", readReport), &report); report.State != "awaiting_approval" {
		t.Fatalf("read_import answered %q after the test, want awaiting_approval", report.State)
	}
}

func TestASecondPreviewByOneAgentAnswersItsRun(t *testing.T) {
	d := newTwoDoors(t, "preview-agent-busy", "read", "write")
	var first waitedPreview
	decode(t, d.tool(t, "preview_import", previewArgs("First GmbH")), &first)

	refusal := d.toolRefused(t, "preview_import", previewArgs("Second GmbH"))
	if !strings.Contains(refusal, first.Run.RunID) || !strings.Contains(refusal, "read_import that run") {
		t.Errorf("the refusal = %q, want it to send the caller to read_import run %s", refusal, first.Run.RunID)
	}
	if !strings.Contains(refusal, "can succeed later") {
		t.Errorf("the refusal = %q, want it to say the same call can succeed once that test ends", refusal)
	}
}

func TestCommitImportResumesAFailedCommit(t *testing.T) {
	d := newTwoDoors(t, "commit-resumes", "read", "write")
	run := previewCompanies(t, d, "").Run.RunID
	d.tool(t, "commit_import", map[string]any{"run_id": run})
	storeless, err := compose.ImportJobWorkersForTest(d.e.Pool, compose.JobRunnerConfig{})
	if err != nil {
		t.Fatalf("building the workers of a role with no object store: %v", err)
	}
	results := importjobtest.WorkQueuedBy(t, d.e, storeless, compose.ImportCommitArgs{}.Kind(), 0)
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("the commit ended %+v, want its job completed with the run failed", results)
	}
	if failed := readImportRun(t, d.e, run); failed.Status != "failed" {
		t.Fatalf("the run is %q, want failed", failed.Status)
	}

	var resumed waitedPreview
	decode(t, d.tool(t, "commit_import", map[string]any{"run_id": run}), &resumed.Run)
	if resumed.Run.State != "running" {
		t.Fatalf("commit_import on the failed run answered %q, want it running again", resumed.Run.State)
	}
	importjobtest.Drain(t, d.e)
	if done := readImportRun(t, d.e, run); done.Status != "complete" {
		t.Fatalf("the resumed run is %q %q, want complete", done.Status, done.Error)
	}
}
