// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What a settled dossier tells the AI-activity projection about itself.
//
// The subject is SiteRead.activityState, and what it decides is whether the
// agent rail's orb goes amber: the projection's fault arm is bounded on
// `degraded` and `failed`, and the rail holds the newest unacknowledged one
// until a reader opens the panel. So the question every case below asks is the
// product question — is this an outcome a person has to be told about — and
// not merely which word the row carries.

import (
	"testing"
	"time"
)

// partialAt builds the settled row a crawl leaves behind when it stopped early
// having read pages: `partial` is the only status stopped_reason may accompany.
func partialAt(reason string, warnings ...string) SiteRead {
	return SiteRead{Status: siteReadStatusPartial, StoppedReason: &reason, Warnings: warnings}
}

// A crawl fills the budget it was given. That is the design working, not a
// fault, and it is the common case: an ordinary company site has more pages
// than the cap buys, so reporting it as a fault put an amber orb in front of
// every new reader on their first screen.
func TestAPartialAtTheCrawlsOwnCeilingReportsDone(t *testing.T) {
	for _, reason := range []string{siteReadStopPageCap, siteReadStopByteCap, siteReadStopDeadline} {
		if got := partialAt(reason).activityState(); got != siteReadStatusDone {
			t.Errorf("a crawl stopped at %s reports %q, want %q", reason, got, siteReadStatusDone)
		}
	}
}

// The workspace running out of AI credit is not a ceiling the crawl was built
// to fill: it is a condition an operator repairs, and the reader is owed it.
func TestAPartialAtTheAIBudgetStaysDegraded(t *testing.T) {
	if got := partialAt(siteReadStopBudget).activityState(); got != activityStateDegraded {
		t.Errorf("a crawl stopped at the AI budget reports %q, want %q", got, activityStateDegraded)
	}
}

// No stop reason and still partial means the extraction fan-out died with
// evidence already in hand. Nothing was capped; something broke.
func TestAPartialWithNoCeilingStaysDegraded(t *testing.T) {
	if got := (SiteRead{Status: siteReadStatusPartial}).activityState(); got != activityStateDegraded {
		t.Errorf("a partial with no stop reason reports %q, want %q", got, activityStateDegraded)
	}
}

// The case the stop reason alone cannot see: a run may fill its page budget AND
// lose an extraction lane, and the row spells that with the cap's own reason,
// indistinguishable from a clean stop. The worker's warning is what tells them
// apart, and a fault must not be read as a full budget.
func TestPartialAtACeilingWithALostLaneStaysDegraded(t *testing.T) {
	capped := partialAt(siteReadStopPageCap, SiteReadPartialExtractionWarning)
	if got := capped.activityState(); got != activityStateDegraded {
		t.Errorf("a capped crawl that also lost a lane reports %q, want %q", got, activityStateDegraded)
	}
}

// A caveat that is not the extraction one must not suppress the ceiling: the
// legal-census and client-rendered-site notes are things the dossier says about
// what it found, not reports that the read went wrong.
func TestOtherCaveatsDoNotTurnACeilingIntoAFault(t *testing.T) {
	capped := partialAt(siteReadStopPageCap, "This site builds its pages in the browser.")
	if got := capped.activityState(); got != siteReadStatusDone {
		t.Errorf("a capped crawl carrying an unrelated caveat reports %q, want %q", got, siteReadStatusDone)
	}
}

// A deferral kept what it had read and is waiting on budget that may return
// hours later; a cancellation did not happen at all. Neither word changes here,
// and pinning them is what keeps the ceiling split above from widening into
// outcomes it was never about.
func TestTheOtherSettledStatusesKeepTheirWords(t *testing.T) {
	for status, want := range map[string]string{
		siteReadStatusDeferred:  activityStateDegraded,
		siteReadStatusCancelled: siteReadStatusFailed,
		siteReadStatusFailed:    siteReadStatusFailed,
		siteReadStatusDone:      siteReadStatusDone,
		siteReadStatusQueued:    siteReadStatusQueued,
		siteReadStatusRunning:   siteReadStatusRunning,
	} {
		if got := (SiteRead{Status: status}).activityState(); got != want {
			t.Errorf("%s reports %q, want %q", status, got, want)
		}
	}
}

// A ceiling stop reports `done` and still says why it ended where it did. The
// state decides whether a reader is interrupted; the reason is the record, and
// dropping it would buy the calm orb by making the occurrence say less than the
// row knows.
func TestACeilingStopStillCarriesItsReason(t *testing.T) {
	capped := partialAt(siteReadStopPageCap)
	if got := capped.activityDegradeReason(); got != siteReadStopSaid[siteReadStopPageCap] {
		t.Errorf("a page-capped crawl explains itself with %q, want %q", got, siteReadStopSaid[siteReadStopPageCap])
	}
}

// A settled attempt says WHEN it settled whatever word it settled under. The
// ceiling split moved a partial from `degraded` to `done`, and a `done`
// occurrence with no finished_at would read as still running.
func TestACeilingStopIsStillSettled(t *testing.T) {
	finished := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	capped := partialAt(siteReadStopPageCap)
	capped.FinishedAt = &finished
	if got := capped.activitySettledAt(); got == nil || !got.Equal(finished) {
		t.Errorf("a page-capped crawl settled at %v, want %v", got, finished)
	}
}
