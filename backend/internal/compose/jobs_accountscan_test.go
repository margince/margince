// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/riverqueue/river"
)

// One read per (account, reader) sits in the queue at a time: a second open
// while one is queued must fold into it rather than start a rival.
//
// The lane it lands on is not asserted here — jobs.QueuedAs supplies it from
// the declaration, and TestNoInsertOptsNamesItsOwnQueue keeps it that way.
func TestAnAccountScanIsQueuedOnceOnTheTranscriptLane(t *testing.T) {
	if opts := accountScanInsertOpts(); !opts.UniqueOpts.ByArgs {
		t.Errorf("insert opts = %+v, want unique by args", opts)
	}
}

// Every reading a door re-arms after its worker died queues the replacement
// under the SAME read id as the job that finished without it. River's default
// uniqueness window counts finished jobs, so anything wider than the active
// states swallows the replacement and leaves the re-armed row queued with
// nothing behind it — the strand the re-arm exists to end.
func TestARearmableReadingDedupesItsJobOnlyWhileOneIsStillActive(t *testing.T) {
	for name, opts := range map[string]*river.InsertOpts{
		"account scan":       accountScanInsertOpts(),
		"transcript propose": transcriptProposeInsertOpts(),
	} {
		if !opts.UniqueOpts.ByArgs || !slices.Equal(opts.UniqueOpts.ByState, activeSweepStates) {
			t.Errorf("%s: unique %+v; want by args, over the active states only", name, opts.UniqueOpts)
		}
	}
}
