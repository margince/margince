// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
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

// A job that names no workspace cannot bind a reader's authority, and the
// carrier is told so as a fault rather than left to retry into the same
// refusal.
func TestAScanJobWithoutAWorkspaceIsAFault(t *testing.T) {
	w := &accountScanWorker{log: slog.Default()}
	err := w.Work(context.Background(), &river.Job[AccountScanArgs]{Args: AccountScanArgs{
		OrganizationID: ids.NewV7(), ScanID: ids.NewV7(), ViewerID: ids.NewV7(),
	}})
	if err == nil {
		t.Fatal("a scan job with no workspace was worked")
	}
}
