// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// declaredTimeout is what River will be handed for a kind, resolved exactly as
// addGovernedWorker resolves it. Reading the Spec rather than a worker is the
// point of this commit: no worker answers the question any more.
func declaredTimeout(t *testing.T, kind string) time.Duration {
	t.Helper()
	spec, ok := jobs.SpecFor(kind)
	if !ok {
		t.Fatalf("%s is not declared in api/jobs.yaml", kind)
	}
	return spec.Timeout.Duration()
}

// TestTheTwoPassesDeclaredWithNoWallClockStillResolveToNegativeOne pins the
// behaviour the deleted Timeout methods used to hold. A NEGATIVE River timeout
// is not "a long time": job_rescuer.go ignores a stuck job whose worker
// declares one, at any age, which is the wedge search.ReembedClaim.StealAfter
// exists to answer. A {none: true} declaration that resolved to 0 instead
// would restore River's one-minute default on the two longest passes in the
// tree and re-arm the rescuer behind them, and neither change would be visible
// anywhere else.
//
// The unbounded half is whatever actually WALKS a corpus. Both do now:
// embed_reindex absorbed the child it used to fan out to when ADR-0091 §8 phase
// D removed the tenant column, and embed_drift_sweep absorbed its own under
// ADR-0103 — so the kind that was asserted BOUNDED here, on the ground that a
// dispatcher only enumerates and enqueues, is the same kind that now does the
// walk. It changed sides rather than leaving, which is the whole content of the
// collapse: there is no longer a row whose deadline can be short because it
// does nothing.
func TestTheTwoPassesDeclaredWithNoWallClockStillResolveToNegativeOne(t *testing.T) {
	for _, kind := range []string{"embed_reindex", "embed_drift_sweep"} {
		if got := declaredTimeout(t, kind); got != -1 {
			t.Errorf("%s resolves to %v, want -1 — the pass is bounded by its backlog, and the row must stay outside River's rescuer", kind, got)
		}
	}
}

// TestTheDeepReadTimeoutOutlastsTheLongestCrawlAnAdminMayChoose: the wall is a
// setting read when a read starts, while River fixes the job's timeout when the
// worker registers. A timeout sized for anything less than the ceiling would
// let an admin choose a crawl River kills before its dossier is written.
func TestTheDeepReadTimeoutOutlastsTheLongestCrawlAnAdminMayChoose(t *testing.T) {
	longest := time.Duration(capture.MaxSiteReadWallSeconds)*time.Second + extractLaneBudget + logoLaneBudget
	if got, want := declaredTimeout(t, "site_deep_read"), longest+time.Minute; got < want {
		t.Errorf("site_deep_read runs under %v, want at least %v: the longest allowed crawl, its extraction and a minute for the dossier writes", got, want)
	}
	if reclaim := (&siteDeepReadWorker{}).reclaimAfter(); reclaim <= declaredTimeout(t, "site_deep_read") {
		t.Errorf("a stuck read is reclaimed after %v, inside the job's own timeout — a live worker could lose its read", reclaim)
	}
}

// TestAddGovernedWorkerRecordsTheKindItRegistered is what makes MustBeTotal at
// boot mean anything: River keeps its own registry unexported, so a kind that
// went in without being recorded here would be invisible to the totality check
// and would run at whatever default it inherited.
func TestAddGovernedWorkerRecordsTheKindItRegistered(t *testing.T) {
	reg := newJobRegistry()
	addGovernedWorker[CloseDateSweepArgs](reg, &closeDateSweepWorker{})
	addGovernedWorker[GmailWatchRenewArgs](reg, &gmailWatchRenewWorker{})

	want := []string{"close_date_sweep", "gmail_watch_renew_connection"}
	if len(reg.kinds) != len(want) {
		t.Fatalf("recorded %v, want %v", reg.kinds, want)
	}
	for i, kind := range want {
		if reg.kinds[i] != kind {
			t.Errorf("recorded kind %d = %q, want %q", i, reg.kinds[i], kind)
		}
	}
	if err := jobs.MustBeTotal(reg.kinds); err != nil {
		t.Errorf("the kinds the runner registers must all be declared: %v", err)
	}
}
