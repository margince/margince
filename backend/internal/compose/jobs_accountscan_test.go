// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"slices"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// One read per (account, reader) sits in the queue at a time, on the
// transcript lane it shares: a second open while one is queued must fold
// into it rather than start a rival.
func TestAnAccountScanIsQueuedOnceOnTheTranscriptLane(t *testing.T) {
	opts := accountScanInsertOpts()
	if opts.Queue != accountScanQueue || !opts.UniqueOpts.ByArgs {
		t.Errorf("insert opts = %+v, want the transcript queue, unique by args", opts)
	}
}

// Every reading a door re-arms after its worker died queues the replacement
// under the SAME read id as the job that finished without it. River's default
// uniqueness window counts finished jobs, so anything wider than the active
// states swallows the replacement and leaves the re-armed row queued with
// nothing behind it — the strand the re-arm exists to end.
func TestARearmableReadingDedupesItsJobOnlyWhileOneIsStillActive(t *testing.T) {
	for name, opts := range map[string]*river.InsertOpts{
		"document extract":   documentExtractInsertOpts(),
		"account scan":       accountScanInsertOpts(),
		"transcript propose": transcriptProposeInsertOpts(),
	} {
		if !opts.UniqueOpts.ByArgs || !slices.Equal(opts.UniqueOpts.ByState, activeSweepStates) {
			t.Errorf("%s: unique %+v; want by args, over the active states only", name, opts.UniqueOpts)
		}
	}
}

// A job that names no workspace cannot bind a reader's authority, and the
// carrier is told so as a fault rather than left to retry into the same
// refusal.
func TestAScanJobWithoutAWorkspaceIsAFault(t *testing.T) {
	w := &accountScanWorker{log: slog.Default()}
	err := w.Work(context.Background(), &river.Job[AccountScanArgs]{Args: AccountScanArgs{
		CompanyID: ids.NewV7(), ScanID: ids.NewV7(), ViewerID: ids.NewV7(),
	}})
	if err == nil {
		t.Fatal("a scan job with no workspace was worked")
	}
}
