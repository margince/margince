// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The numbers that bound an import's work in the worker. A commit or an undo
// runs as slices of one attempt. Each slice must end, flush and snooze inside
// the window a stopping worker gives it.

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
)

const (
	// importSliceRows ends a slice by rows.
	importSliceRows = 500
	// importSliceBudget ends a slice by time. With setup and the last flush it
	// stays under the worker's drain window, so a shutdown loses no slice.
	importSliceBudget = 12 * time.Second
	// importSliceTimeout is the wall clock of one slice job. Past it a
	// statement is stuck, and the run fails over budget.
	importSliceTimeout = time.Minute
	// importFlushEvery and importFlushRows are how often a slice writes its
	// progress before it ends.
	importFlushEvery = 2 * time.Second
	importFlushRows  = 500
	// importStopFlushTimeout bounds the write that records a stopped slice. The
	// engine flushes its own leg under the same bound.
	importStopFlushTimeout = migration.StopFlushTimeout
	// importStaleAfter is when a claimed run's silence means its process died.
	// A snoozed slice waiting behind another run is never that quiet.
	importStaleAfter = 2 * importSliceTimeout
	// importQueuedReinsertAfter is when a queued run whose job never started is
	// queued again under the same attempt.
	importQueuedReinsertAfter = 2 * time.Minute
	// importMaxResumes bounds the attempts the sweep starts for a stale run.
	importMaxResumes = 3
	// importLockWait is how long a slice snoozes while another holds its run.
	importLockWait = 5 * time.Second
	// importDraftTTL is how long a draft or a report keeps its uploaded file
	// while nobody touches it.
	importDraftTTL = 7 * 24 * time.Hour
	// importRowCap is the most rows one upload may hold, summed over its files,
	// because one test run reads them all. A mixed company, contact and lead
	// file was read at about 1,000 rows a second on a developer machine. The
	// cap keeps a third of the test run's budget for matching the estate.
	importRowCap = 15_000
)

const (
	// importWriteQueue keeps commit and undo slices apart from the jobs a
	// user is waiting on. Each slice holds the run's lock connection plus one
	// row transaction at a time.
	importWriteQueue = "import_write"
	// One worker per process: slices of two runs interleave through the
	// snooze, so one worker is fair to both.
	importWriteMaxWorkers = 1
)

// importSliceBudgetFrom is the budget of a slice that starts at now.
func importSliceBudgetFrom(now time.Time) migration.Budget {
	return migration.Budget{
		Rows:       importSliceRows,
		Until:      now.Add(importSliceBudget),
		FlushRows:  importFlushRows,
		FlushEvery: importFlushEvery,
	}
}
