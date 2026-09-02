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
	if len(sites) == 0 {
		t.Fatal("the census is empty, so the verification below would report a pass " +
			"having checked nothing — which is the exact failure this test defends against")
	}
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

// A lower rung bound to the SAME model is not a fallback a reader can act on.
// The row already reports that model, and naming it again as "a fallback we
// have not checked" contradicts the line directly above it — which is what the
// first run of this card against a real deployment showed.
func TestTheServingModelIsNotItsOwnUnmeasuredFallback(t *testing.T) {
	t.Parallel()

	// draft_reply's ladder is [cheap_cloud, premium] and cheap_cloud degrades to
	// local_small, so the closure reaches three rungs — here all one model.
	routing := ai.RoutingConfig{
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierCheapCloud: {Provider: testProvider, Model: testModel},
			ai.TierPremium:    {Provider: testProvider, Model: testModel},
			ai.TierLocalSmall: {Provider: testProvider, Model: testModel},
		},
		Profile: ai.Profile(testEnv),
	}
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	job := jobNamed(t, certificationView(routing, sites, snapOf(t)), ai.TaskDraftReply)

	if job.UnmeasuredFallbacks != nil {
		t.Errorf("unmeasured_fallbacks = %v; every rung binds the model the row already names",
			*job.UnmeasuredFallbacks)
	}
}

// worst_site is evidence for a measurement. On a job nothing measured, every
// site reads the same, and naming one implies it was the reason — sending a
// reader to look for a finding that is not there.
func TestNothingMeasuredNamesNoWorstSite(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{
		siteOf(ai.TaskColdStart, "acts"),
		siteOf(ai.TaskColdStart, "company_message"),
	}
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snapOf(t)), ai.TaskColdStart)

	if job.Result != resultNotChecked {
		t.Fatalf("result = %q, want %q", job.Result, resultNotChecked)
	}
	if job.WorstSite != nil {
		t.Errorf("worst_site = %q on a job nothing measured", *job.WorstSite)
	}
}

// "Out of date" and "partly checked" describe a measurement's STANDING, not its
// finding. Two committed rows are stale not_supported at 12 of 12 runs passed —
// so a card that dropped the verdict and kept the counts would report a
// measurement whose conclusion was "do not trust this" as an old perfect score.
func TestAStaleFailureKeepsItsVerdictAndNotJustItsCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status string
		want   crmcontracts.AiCertificationResult
	}{
		{"out of date", snapshot.StatusStale, resultOutOfDate},
		{"partly checked", snapshot.StatusPartial, resultPartlyChecked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
			snap := snapOf(t, rowOf("draft_reply", "reply", tc.status, "not_supported", 12, 12, 4, 1))
			job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)

			if job.Result != tc.want {
				t.Errorf("result = %q, want %q", job.Result, tc.want)
			}
			if job.MeasuredResult == nil {
				t.Fatal("the measurement's own finding was dropped, leaving only its counts")
			}
			if *job.MeasuredResult != resultNotReliable {
				t.Errorf("measured_result = %q, want %q", *job.MeasuredResult, resultNotReliable)
			}
		})
	}
}

// A stale row carrying no band has no verdict to report beside its age, and
// "out of date — it was not checked" is noise rather than evidence.
func TestAStaleRowWithNoVerdictReportsNoFinding(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	snap := snapOf(t, rowOf("draft_reply", "reply", snapshot.StatusStale, "", 0, 0, 0, 0))
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)

	if job.MeasuredResult != nil {
		t.Errorf("measured_result = %q on a row that measured nothing", *job.MeasuredResult)
	}
}

// A status this build does not know is a row it cannot read. Falling through to
// the band would render it as a reliability finding nobody produced.
func TestAnUnknownStatusFailsClosed(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	snap := snapOf(t, rowOf("draft_reply", "reply", "retracted", "certified", 9, 9, 3, 0))
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)

	if job.Result != resultNotChecked {
		t.Errorf("result = %q on an unreadable status, want %q", job.Result, resultNotChecked)
	}
}

// A fallback measured six weeks ago HAS been checked. The list says "which we
// have not checked", so announcing a stale measurement there is false.
func TestAStaleFallbackIsNotCalledUnchecked(t *testing.T) {
	t.Parallel()

	routing := ai.RoutingConfig{
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierCheapCloud: {Provider: testProvider, Model: testModel},
			ai.TierLocalSmall: {Provider: testProvider, Model: "some/older-model"},
		},
		Profile: ai.Profile(testEnv),
	}
	stale := rowOf("draft_reply", "reply", snapshot.StatusStale, "certified", 9, 9, 3, 0)
	stale.Model = "some/older-model"
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	snap := snapOf(
		t,
		rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 9, 9, 3, 0),
		stale,
	)
	job := jobNamed(t, certificationView(routing, sites, snap), ai.TaskDraftReply)

	if job.UnmeasuredFallbacks != nil {
		t.Errorf("unmeasured_fallbacks = %v; that model was measured, only not recently",
			*job.UnmeasuredFallbacks)
	}
}

// The sample size comes from the row's own counts. The lane's repeat count is
// configurable per run, so a figure copied from the runner would be a claim
// about runs the record never saw.
func TestTheSampleSizeIsDerivedFromTheRowsOwnCounts(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	// 25 runs over 5 examples is five runs each — not the lane's default of 3.
	snap := snapOf(t, rowOf("draft_reply", "reply", snapshot.StatusCurrent, "certified", 25, 25, 5, 0))
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)

	if job.RunsPerExample == nil || *job.RunsPerExample != 5 {
		t.Errorf("runs_per_example = %v, want 5 from 25 runs over 5 examples", job.RunsPerExample)
	}
}

// worst_site is evidence only where the fold had to choose. On a job whose sites
// all read the same, naming one implies it was the reason and a reader goes
// looking for a finding that is not there. Ties pick the last site in census
// order, which is arbitrary.
func TestSitesThatAgreeNameNoWorstSite(t *testing.T) {
	t.Parallel()

	sites := []aitasks.Site{
		siteOf(ai.TaskColdStart, "acts"),
		siteOf(ai.TaskColdStart, "company_message"),
	}
	snap := snapOf(
		t,
		rowOf("cold_start", "acts", snapshot.StatusCurrent, "certified", 9, 9, 3, 0),
		rowOf("cold_start", "company_message", snapshot.StatusCurrent, "certified", 9, 9, 3, 0),
	)
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskColdStart)

	if job.Result != resultReliable {
		t.Fatalf("result = %q, want %q", job.Result, resultReliable)
	}
	if job.WorstSite != nil {
		t.Errorf("worst_site = %q where every site read the same", *job.WorstSite)
	}
}
