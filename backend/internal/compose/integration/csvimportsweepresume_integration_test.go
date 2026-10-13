// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// attemptRecord is the run row's record of its current attempt.
type attemptRecord struct {
	attempt, resumes int
	correlation      string
}

func readLease(t *testing.T, e *apptest.AppEnv, runID string) attemptRecord {
	t.Helper()
	var l attemptRecord
	if err := e.Owner.QueryRow(t.Context(), `
		SELECT job_attempt, resume_count, coalesce(attempt_correlation_id::text, '')
		  FROM import_run WHERE id = $1`, runID).Scan(&l.attempt, &l.resumes, &l.correlation); err != nil {
		t.Fatalf("reading the run's lease: %v", err)
	}
	return l
}

// attemptJob is one River row queued for a run: the attempt it names and its state.
type attemptJob struct {
	attempt int
	state   string
}

func jobsOf(t *testing.T, e *apptest.AppEnv, kind, runID string) []attemptJob {
	t.Helper()
	rows, err := e.Owner.Query(t.Context(), `
		SELECT (args->>'attempt')::int, state::text FROM river_job
		 WHERE kind = $1 AND args->>'run_id' = $2 ORDER BY id`, kind, runID)
	if err != nil {
		t.Fatalf("listing the run's jobs: %v", err)
	}
	defer rows.Close()
	var listed []attemptJob
	for rows.Next() {
		var j attemptJob
		if err := rows.Scan(&j.attempt, &j.state); err != nil {
			t.Fatalf("reading the run's jobs: %v", err)
		}
		listed = append(listed, j)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing the run's jobs: %v", err)
	}
	return listed
}

// claimThenFallSilent makes the run's attempt claimed with a heartbeat an hour
// old, as when the process working it died, after resumes earlier resumes.
func claimThenFallSilent(t *testing.T, e *apptest.AppEnv, runID string, resumes int) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(), `
		UPDATE import_run SET claimed_attempt = job_attempt, heartbeat_at = now() - interval '1 hour',
		       resume_count = $2
		 WHERE id = $1`, runID, resumes); err != nil {
		t.Fatalf("claiming the run's attempt: %v", err)
	}
}

// killJobs leaves the run's jobs of kind running under a worker that died an
// hour ago. Nothing rescues such a row.
func killJobs(t *testing.T, e *apptest.AppEnv, kind, runID string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(), `
		UPDATE river_job SET state = 'running', attempt = 1, attempted_at = now() - interval '1 hour'
		 WHERE kind = $1 AND args->>'run_id' = $2`, kind, runID); err != nil {
		t.Fatalf("killing the run's jobs: %v", err)
	}
}

// ageQueuedRun makes the run's last write older than the queued-job window.
func ageQueuedRun(t *testing.T, e *apptest.AppEnv, runID string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET updated_at = now() - interval '1 hour' WHERE id = $1`, runID); err != nil {
		t.Fatalf("ageing the run: %v", err)
	}
}

// approvedButUnworked answers a run whose commit job is queued and never worked.
func approvedButUnworked(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	return run.ID
}

func TestAQueuedRunIsNotSwept(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	runID := approvedButUnworked(t, e)
	queued := readLease(t, e, runID)
	// Older than every window the sweep has: the job is only waiting its turn.
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET updated_at = now() - interval '2 hours' WHERE id = $1`, runID); err != nil {
		t.Fatalf("ageing the run: %v", err)
	}

	sweepImportRuns(t, e)
	if polled := readRun(t, e, runID); polled.Status != "running" || polled.Error != "" {
		t.Fatalf("the sweep moved a queued run to %q %q", polled.Status, polled.Error)
	}
	if got := readLease(t, e, runID); got.attempt != queued.attempt || got.resumes != 0 {
		t.Fatalf("the sweep left the run at attempt %d with %d resumes, want attempt %d with none",
			got.attempt, got.resumes, queued.attempt)
	}
	if got := jobsOf(t, e, commit, runID); !slices.Equal(got, []attemptJob{{queued.attempt, "available"}}) {
		t.Fatalf("the run's jobs are %v, want its one queued job", got)
	}

	importjobtest.Drain(t, e)
	if polled := readRun(t, e, runID); polled.Status != "complete" || leadCount(t, e) != 3 {
		t.Fatalf("the queued job ended the run %q with %d leads, want complete with 3", polled.Status, leadCount(t, e))
	}
}

func TestAStaleClaimedRunIsRequeuedAsANewAttempt(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	runID := approvedButUnworked(t, e)
	claimThenFallSilent(t, e, runID, 0)
	stale := readLease(t, e, runID)

	sweepImportRuns(t, e)
	got := readLease(t, e, runID)
	if got.attempt != stale.attempt+1 || got.resumes != 1 {
		t.Fatalf("after the sweep the run is at attempt %d with %d resumes, want attempt %d with 1",
			got.attempt, got.resumes, stale.attempt+1)
	}
	if got.correlation == "" || got.correlation == stale.correlation {
		t.Fatalf("the new attempt traces as %q, want a correlation of its own (the stale one was %q)",
			got.correlation, stale.correlation)
	}
	want := []attemptJob{{stale.attempt, "available"}, {stale.attempt + 1, "available"}}
	if jobs := jobsOf(t, e, commit, runID); !slices.Equal(jobs, want) {
		t.Fatalf("the run's jobs are %v, want %v", jobs, want)
	}

	// The stale attempt's job is fenced off; only the new one lands rows.
	importjobtest.Drain(t, e)
	if polled := readRun(t, e, runID); polled.Status != "complete" || leadCount(t, e) != 3 {
		t.Fatalf("the requeued run ended %q with %d leads, want complete with 3", polled.Status, leadCount(t, e))
	}
}

// A run whose slice snoozed waits its turn behind other runs. However old its
// heartbeat, the stale rule never resumes it, and its own job finishes it.
func TestASnoozedRunIsNeverStaleHoweverOldItsHeartbeat(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	run := testedLeadRun(t, e, leadFile(501))
	approveQueued(t, e, run.ID)
	if worked := importjobtest.WorkQueued(t, e, commit); len(worked) != 1 {
		t.Fatalf("the first slice worked %d jobs, want one", len(worked))
	}
	snoozed := readLease(t, e, run.ID)
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET heartbeat_at = now() - interval '1 hour' WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("ageing the heartbeat: %v", err)
	}

	sweepImportRuns(t, e)
	if got := readLease(t, e, run.ID); got != snoozed {
		t.Fatalf("the sweep moved the snoozed run to %+v, want it as the slice left it, %+v", got, snoozed)
	}
	importjobtest.Drain(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "complete" || storedLeads(t, e) != 501 {
		t.Fatalf("the snoozed job ended the run %q with %d leads, want complete with 501", polled.Status, storedLeads(t, e))
	}
}

func TestADeadRiverRowDoesNotBlockTheRequeue(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	runID := approvedButUnworked(t, e)
	killJobs(t, e, commit, runID)
	claimThenFallSilent(t, e, runID, 0)

	sweepImportRuns(t, e)
	worked := importjobtest.WorkQueued(t, e, commit)
	if len(worked) != 1 {
		t.Fatalf("one sweep tick queued %d commit jobs, want the new attempt's one", len(worked))
	}
	if polled := readRun(t, e, runID); polled.Status != "complete" || leadCount(t, e) != 3 {
		t.Fatalf("the new attempt ended the run %q with %d leads, want complete with 3", polled.Status, leadCount(t, e))
	}
}

func TestAJobLostBeforeItsClaimIsReinsertedOnce(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	runID := approvedButUnworked(t, e)
	queued := readLease(t, e, runID)
	if _, err := e.Owner.Exec(t.Context(), `
		UPDATE river_job SET state = 'discarded', finalized_at = now()
		 WHERE kind = $1 AND args->>'run_id' = $2`, commit, runID); err != nil {
		t.Fatalf("discarding the run's job: %v", err)
	}

	want := []attemptJob{{queued.attempt, "discarded"}, {queued.attempt, "available"}}
	for pass := range 2 {
		ageQueuedRun(t, e, runID)
		sweepImportRuns(t, e)
		if got := readLease(t, e, runID); got.attempt != queued.attempt || got.resumes != 1 {
			t.Fatalf("pass %d left the run at attempt %d with %d resumes, want attempt %d with 1",
				pass+1, got.attempt, got.resumes, queued.attempt)
		}
		if jobs := jobsOf(t, e, commit, runID); !slices.Equal(jobs, want) {
			t.Fatalf("after pass %d the run's jobs are %v, want %v", pass+1, jobs, want)
		}
	}

	importjobtest.Drain(t, e)
	if polled := readRun(t, e, runID); polled.Status != "complete" || leadCount(t, e) != 3 {
		t.Fatalf("the reinserted job ended the run %q with %d leads, want complete with 3", polled.Status, leadCount(t, e))
	}
}

func TestPastTheResumeLimitTheRunFails(t *testing.T) {
	e := setupImportApp(t)
	commit, undo := compose.ImportCommitArgs{}.Kind(), compose.ImportUndoArgs{}.Kind()

	t.Run("a commit fails", func(t *testing.T) {
		runID := approvedButUnworked(t, e)
		killJobs(t, e, commit, runID)
		claimThenFallSilent(t, e, runID, 3)
		stale := readLease(t, e, runID)

		sweepImportRuns(t, e)
		if polled := readRun(t, e, runID); polled.Status != "failed" || polled.Error != migration.CauseStale {
			t.Fatalf("past the limit the run is %q %q, want failed with %q", polled.Status, polled.Error, migration.CauseStale)
		}
		if jobs := jobsOf(t, e, commit, runID); !slices.Equal(jobs, []attemptJob{{stale.attempt, "running"}}) {
			t.Fatalf("past the limit the run's jobs are %v, want no new one", jobs)
		}
	})

	t.Run("an undo keeps undoing until undone again", func(t *testing.T) {
		profile, _ := uploadCSV(t, e, "lead", prospectCSV)
		run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
		approveAndRead(t, e, run.ID)
		if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, nil); status != http.StatusAccepted {
			t.Fatalf("undo → %d, want 202", status)
		}
		killJobs(t, e, undo, run.ID)
		claimThenFallSilent(t, e, run.ID, 3)
		stale := readLease(t, e, run.ID)

		for pass := range 2 {
			ageQueuedRun(t, e, run.ID)
			sweepImportRuns(t, e)
			if polled := readRun(t, e, run.ID); polled.Status != "undoing" || polled.Error != migration.CauseUndoStale {
				t.Fatalf("pass %d left the undo %q %q, want undoing with %q", pass+1, polled.Status, polled.Error, migration.CauseUndoStale)
			}
			if got := readLease(t, e, run.ID); got.attempt != stale.attempt || got.resumes != 3 {
				t.Fatalf("pass %d moved the undo to attempt %d with %d resumes; a stopped undo waits for a human",
					pass+1, got.attempt, got.resumes)
			}
		}
		var rewritten bool
		if err := e.Owner.QueryRow(t.Context(), `SELECT updated_at > now() - interval '1 minute' FROM import_run WHERE id = $1`,
			run.ID).Scan(&rewritten); err != nil || rewritten {
			t.Fatalf("the second pass wrote the stopped undo again (%v): it must leave it to a human", err)
		}
		if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
			t.Fatalf("undo again → %d, want 202", status)
		}
		if polled := readRun(t, e, run.ID); polled.Status != "undone" {
			t.Fatalf("the undo asked again ended %q, want undone", polled.Status)
		}
	})
}
