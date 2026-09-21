// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// TestSiteDeepReadInsertOptsRoutesQueueAndCarriesTheGivenPriority holds what
// every site_deep_read caller depends on: the job always lands on
// deepReadQueue, deduplicates by args, and carries whichever priority the
// caller declared for itself — and that DeepReadPriorityLive is fetched
// before DeepReadPriorityHousekeeping, not the other way around.
func TestSiteDeepReadInsertOptsRoutesQueueAndCarriesTheGivenPriority(t *testing.T) {
	for _, priority := range []int{DeepReadPriorityLive, DeepReadPriorityHousekeeping} {
		opts := siteDeepReadInsertOpts(priority)
		if opts.Queue != deepReadQueue {
			t.Errorf("priority %d: queue = %q, want %q", priority, opts.Queue, deepReadQueue)
		}
		if opts.Priority != priority {
			t.Errorf("priority %d: opts.Priority = %d, want it carried through unchanged", priority, opts.Priority)
		}
		if !opts.UniqueOpts.ByArgs {
			t.Errorf("priority %d: UniqueOpts.ByArgs = false, want deduplication by the dossier id in args", priority)
		}
	}
	if DeepReadPriorityLive == DeepReadPriorityHousekeeping {
		t.Fatal("live and housekeeping priorities are equal — a boot sweep would queue level with a live read again")
	}
	if DeepReadPriorityLive >= DeepReadPriorityHousekeeping {
		t.Fatalf("DeepReadPriorityLive (%d) must be fetched before DeepReadPriorityHousekeeping (%d) — River treats a LOWER number as higher priority",
			DeepReadPriorityLive, DeepReadPriorityHousekeeping)
	}
}
