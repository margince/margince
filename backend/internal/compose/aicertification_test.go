// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
)

const (
	testProvider = "openai_compatible"
	testModel    = "openai/gpt-oss-120b"
	testEnv      = "eu_hosted"
)

// boundEverywhere binds the test model to every rung, which is the ordinary
// shape of a routing that has been configured at all.
func boundEverywhere() ai.RoutingConfig {
	tiers := map[ai.Tier]ai.ProviderConfig{}
	for _, tier := range ai.AllTiers() {
		tiers[tier] = ai.ProviderConfig{Provider: testProvider, Model: testModel}
	}
	return ai.RoutingConfig{Tiers: tiers, Profile: ai.Profile(testEnv)}
}

func siteOf(task ai.Task, variant string) aitasks.Site {
	return aitasks.Site{Task: task, Variant: variant}
}

func rowOf(task, site, status, band string, runs, passed, measured, pending int) snapshot.Row {
	return snapshot.Row{
		Task: task, Site: site, Provider: testProvider, Model: testModel, EnvClass: testEnv,
		Status: status, Band: band, Scope: "full_invocation",
		Runs: runs, Passed: passed, Measured: measured, Pending: pending,
		RanAt: "2026-09-01T12:00:00Z",
	}
}

func snapOf(t *testing.T, rows ...snapshot.Row) snapshot.Snapshot {
	t.Helper()
	snap, err := snapshot.FromRows(rows)
	if err != nil {
		t.Fatalf("building the test snapshot: %v", err)
	}
	return snap
}

func jobNamed(t *testing.T, view crmcontracts.AiCertification, task ai.Task) crmcontracts.AiCertificationJob {
	t.Helper()
	for _, job := range view.Jobs {
		if job.Task == string(task) {
			return job
		}
	}
	t.Fatalf("no job for %s in the view", task)
	return crmcontracts.AiCertificationJob{}
}

// An installation nobody has configured is not an installation whose models went
// unmeasured. Reporting the first as the second blames the certification lane
// for an empty settings page and sends a reader looking for a run that would not
// help them.
func TestAnUnboundInstallationSaysSoRatherThanReportingNothingMeasured(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	view := certificationView(ai.RoutingConfig{}, sites, snapOf(t))

	if view.BindingState != "unbound" {
		t.Errorf("binding_state = %q, want unbound", view.BindingState)
	}
	job := jobNamed(t, view, ai.TaskDraftReply)
	if job.Result != resultNoModel {
		t.Errorf("result = %q, want %q", job.Result, resultNoModel)
	}
	if job.Model != nil || job.Provider != nil {
		t.Error("a job with nothing bound named a model")
	}
	if len(job.Sites) != 1 || job.Sites[0].Result != resultNoModel {
		t.Errorf("the site must read no_model too, got %+v", job.Sites)
	}
}

// One job unbound while others are bound is a gap in that job's ladder, not a
// failure of the page: the rest still resolve.
func TestOneUnboundJobDoesNotSilenceTheRest(t *testing.T) {
	t.Parallel()

	// capture_confidentiality_verdict's ladder is [local_small] only; draft_reply's
	// is [cheap_cloud, premium]. Binding premium alone leaves the first with no rung.
	routing := ai.RoutingConfig{
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierPremium: {Provider: testProvider, Model: testModel}},
		Profile: ai.Profile(testEnv),
	}
	sites := []aitasks.Site{
		siteOf(ai.TaskCaptureConfidentialityVerdict, "thread"),
		siteOf(ai.TaskDraftReply, "reply"),
	}
	snap := snapOf(t, rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 9, 9, 3, 0))
	view := certificationView(routing, sites, snap)

	if got := jobNamed(t, view, ai.TaskCaptureConfidentialityVerdict).Result; got != resultNoModel {
		t.Errorf("the unbound job = %q, want %q", got, resultNoModel)
	}
	if got := jobNamed(t, view, ai.TaskDraftReply).Result; got != resultReliable {
		t.Errorf("the bound job = %q, want %q", got, resultReliable)
	}
	if view.BindingState != "bound" {
		t.Errorf("binding_state = %q, want bound", view.BindingState)
	}
}

// Each status and band becomes exactly one word, and a high pass rate under a
// failing band stays failing: the verdict folds to the worst example, so 23 of
// 24 can still be not_supported and the card must not smooth that over.
func TestEachStatusAndBandBecomesItsOwnWord(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		status, band   string
		runs, passed   int
		measured, pend int
		want           crmcontracts.AiCertificationResult
	}{
		{"every run passed", snapshot.StatusCurrent, "certified", 9, 9, 3, 0, resultReliable},
		{"measured with failures", snapshot.StatusCurrent, "supported_degraded", 21, 20, 7, 0, resultMostlyReliable},
		{"a high rate under a failing band", snapshot.StatusCurrent, "not_supported", 24, 23, 8, 0, resultNotReliable},
		{"examples it has never seen", snapshot.StatusPartial, "certified", 9, 9, 3, 2, resultPartlyChecked},
		{"prompts it no longer sends", snapshot.StatusStale, "certified", 9, 9, 3, 0, resultOutOfDate},
		{"nothing measured", snapshot.StatusAbsent, "", 0, 0, 0, 0, resultNotChecked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
			snap := snapOf(t, rowOf("draft_reply", "reply", tc.status, tc.band, tc.runs, tc.passed, tc.measured, tc.pend))
			job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)
			if job.Result != tc.want {
				t.Errorf("result = %q, want %q", job.Result, tc.want)
			}
			if tc.status == snapshot.StatusAbsent {
				return
			}
			// The counts travel with the verdict, because the verdict alone
			// cannot say whether 23 of 24 or 1 of 24 produced it.
			if job.Runs == nil || *job.Runs != tc.runs || job.Passed == nil || *job.Passed != tc.passed {
				t.Errorf("counts = %v/%v, want %d/%d", job.Passed, job.Runs, tc.passed, tc.runs)
			}
		})
	}
}

// A job is as trustworthy as its weakest part. Averaging would let three sound
// sites carry one that fails every time, so the fold takes the worst and names
// which site it was.
func TestAJobFoldsToItsWorstSiteAndNamesIt(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{
		siteOf(ai.TaskColdStart, "acts"),
		siteOf(ai.TaskColdStart, "company_message"),
		siteOf(ai.TaskColdStart, "sitereadmessage"),
	}
	snap := snapOf(
		t,
		rowOf("cold_start", "acts", snapshot.StatusCurrent, "certified", 9, 9, 3, 0),
		rowOf("cold_start", "company_message", snapshot.StatusCurrent, "certified", 9, 9, 3, 0),
		rowOf("cold_start", "sitereadmessage", snapshot.StatusCurrent, "not_supported", 9, 6, 3, 0),
	)
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskColdStart)

	if job.Result != resultNotReliable {
		t.Errorf("result = %q, want %q — one failing site sets the job", job.Result, resultNotReliable)
	}
	if job.WorstSite == nil || *job.WorstSite != "sitereadmessage" {
		t.Errorf("worst_site = %v, want sitereadmessage", job.WorstSite)
	}
	if job.Passed == nil || *job.Passed != 6 {
		t.Errorf("the job carries the worst site's own counts, got %v", job.Passed)
	}
	if len(job.Sites) != 3 {
		t.Errorf("the breakdown must keep every site, got %d", len(job.Sites))
	}
}

// A cloud-frontier number says nothing about an EU-hosted binding — but it is
// evidence a reader can go and look at, and reporting it as nothing throws that
// away.
func TestAMeasurementUnderAnotherProfileIsNamedRatherThanCountedOrHidden(t *testing.T) {
	t.Parallel()

	elsewhere := rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 9, 9, 3, 0)
	elsewhere.EnvClass = "cloud_frontier"
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snapOf(t, elsewhere)), ai.TaskDraftReply)

	if job.Result != resultNotChecked {
		t.Errorf("result = %q — another posture's number must not certify this binding", job.Result)
	}
	if job.MeasuredUnderOtherProfile == nil || !*job.MeasuredUnderOtherProfile {
		t.Error("the measurement under another posture was not reported as evidence")
	}
}

// An answer a deployment can actually reach, from a model nobody measured, is a
// gap worth naming — without demoting a job whose everyday model is sound.
func TestAnUnmeasuredFallbackIsACaveatNotADemotion(t *testing.T) {
	t.Parallel()

	// draft_reply's ladder is [cheap_cloud, premium] and cheap_cloud degrades to
	// local_small, so local_small's model can serve it under budget pressure
	// while the ladder never names that rung.
	routing := ai.RoutingConfig{
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierCheapCloud: {Provider: testProvider, Model: testModel},
			ai.TierLocalSmall: {Provider: testProvider, Model: "some/unmeasured-model"},
		},
		Profile: ai.Profile(testEnv),
	}
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	snap := snapOf(t, rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 9, 9, 3, 0))
	job := jobNamed(t, certificationView(routing, sites, snap), ai.TaskDraftReply)

	if job.Result != resultReliable {
		t.Errorf("result = %q — the model that answers today is what the reader asked about", job.Result)
	}
	if job.UnmeasuredFallbacks == nil || len(*job.UnmeasuredFallbacks) != 1 ||
		(*job.UnmeasuredFallbacks)[0] != "some/unmeasured-model" {
		t.Errorf("unmeasured_fallbacks = %v, want the ungraded degrade target", job.UnmeasuredFallbacks)
	}
}

// Production serves the first BOUND rung, not the ladder head. A card that read
// ladder[0] would name a model the router never reaches and then report on a
// deployment nobody is running.
func TestTheJobResolvesToTheRungThatActuallyServes(t *testing.T) {
	t.Parallel()

	// draft_reply leads at cheap_cloud; leave it unbound so premium serves.
	routing := ai.RoutingConfig{
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierPremium: {Provider: testProvider, Model: testModel}},
		Profile: ai.Profile(testEnv),
	}
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	snap := snapOf(t, rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 9, 9, 3, 0))
	job := jobNamed(t, certificationView(routing, sites, snap), ai.TaskDraftReply)

	if job.Tier == nil || *job.Tier != string(ai.TierPremium) {
		t.Errorf("tier = %v, want premium — the first rung the deployment binds", job.Tier)
	}
	if job.Result != resultReliable {
		t.Errorf("result = %q; the served rung's record was not found", job.Result)
	}
}

// The grader is the instrument, not a job the product runs for a user.
func TestTheGraderIsNotOnTheCard(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{siteOf(ai.TaskCertJudge, "judge"), siteOf(ai.TaskDraftReply, "reply")}
	view := certificationView(boundEverywhere(), sites, snapOf(t))
	for _, job := range view.Jobs {
		if job.Task == string(ai.TaskCertJudge) {
			t.Fatal("cert_judge is on the card")
		}
	}
	if len(view.Jobs) != 1 {
		t.Errorf("got %d jobs, want only draft_reply", len(view.Jobs))
	}
}

// The skip list is one entry and must stay one. A census that quietly stops
// covering things reads a smaller tree, reports fine, and has no failing
// assertion to notice.
func TestTheOnlyExcludedJobIsTheGrader(t *testing.T) {
	t.Parallel()

	census, err := NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	sites := census.All()
	view := certificationView(boundEverywhere(), sites, snapOf(t))

	shipped := map[ai.Task]bool{}
	for _, site := range sites {
		shipped[site.Task] = true
	}
	reported := map[string]bool{}
	for _, job := range view.Jobs {
		reported[job.Task] = true
	}
	for task := range shipped {
		if task == certJudge {
			continue
		}
		if !reported[string(task)] {
			t.Errorf("shipped job %s is not on the card, and only the grader may be excluded", task)
		}
	}
}
