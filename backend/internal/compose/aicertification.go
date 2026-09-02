// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Resolving the committed certification results against the binding THIS
// installation runs.
//
// The records say how a model performed a job. The stored routing says which
// model answers that job here. Neither alone answers the question an operator
// has — "can I leave this job unattended?" — and the join is the only place the
// two meet, so it is also the only place the honesty rules live: fold to the
// worst site, never average; keep a real measurement visible even when it is
// old; and never report a choice nobody has made as evidence nobody has
// gathered.

import (
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// certJudge is the certification lane's own grader, not a job the product runs
// for a user. It ships, so the census carries it; it has no place on a screen
// about how the product performs, so it is excluded here — and
// TestTheOnlyExcludedJobIsTheGrader holds that this stays the only exclusion,
// because a skip list nobody guards is how a census quietly stops covering
// things.
const certJudge = ai.TaskCertJudge

// The seven words a job or a site can read as. They are the contract's
// vocabulary, spelled here so the fold below can order them by severity.
const (
	resultReliable       crmcontracts.AiCertificationResult = "reliable"
	resultMostlyReliable crmcontracts.AiCertificationResult = "mostly_reliable"
	resultNotReliable    crmcontracts.AiCertificationResult = "not_reliable"
	resultPartlyChecked  crmcontracts.AiCertificationResult = "partly_checked"
	resultOutOfDate      crmcontracts.AiCertificationResult = "out_of_date"
	resultNotChecked     crmcontracts.AiCertificationResult = "not_checked"
	resultNoModel        crmcontracts.AiCertificationResult = "no_model"
)

// The two states the binding itself can be in, spelled once beside the result
// words for the same reason: a typo is then a compile error rather than a test
// diff nobody reads twice.
const (
	bindingBound   crmcontracts.AiCertificationBindingState = "bound"
	bindingUnbound crmcontracts.AiCertificationBindingState = "unbound"
)

// severity orders the results from soundest to least sound, so folding a job's
// sites is a max over this and not over the words' spelling. Explicit because
// alphabetical order would put `not_checked` above `reliable` and quietly invert
// the fold.
var severity = map[crmcontracts.AiCertificationResult]int{
	resultReliable:       0,
	resultMostlyReliable: 1,
	resultPartlyChecked:  2,
	resultOutOfDate:      3,
	resultNotChecked:     4,
	resultNoModel:        5,
	resultNotReliable:    6,
}

// certificationView joins the committed results to the stored binding.
func certificationView(routing ai.RoutingConfig, sites []aitasks.Site, snap snapshot.Snapshot) crmcontracts.AiCertification {
	view := crmcontracts.AiCertification{
		BindingState: bindingState(routing),
		Jobs:         []crmcontracts.AiCertificationJob{},
	}
	for _, task := range shippedJobs(sites) {
		view.Jobs = append(view.Jobs, jobView(routing, task, sitesOfTask(sites, task), snap))
	}
	return view
}

// bindingState says whether anything is bound at all.
//
// Its own word, because an installation nobody has configured yet is not an
// installation whose models went unmeasured: reporting the first as the second
// blames the certification lane for an empty settings page, and sends a reader
// looking for a run that would not help them.
func bindingState(routing ai.RoutingConfig) crmcontracts.AiCertificationBindingState {
	for _, binding := range routing.Tiers {
		if ai.IsBound(binding) {
			return bindingBound
		}
	}
	return bindingUnbound
}

func shippedJobs(sites []aitasks.Site) []ai.Task {
	seen := map[ai.Task]bool{}
	var out []ai.Task
	for _, site := range sites {
		if site.Task == certJudge || seen[site.Task] {
			continue
		}
		seen[site.Task] = true
		out = append(out, site.Task)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sitesOfTask(sites []aitasks.Site, task ai.Task) []aitasks.Site {
	var out []aitasks.Site
	for _, site := range sites {
		if site.Task == task {
			out = append(out, site)
		}
	}
	return out
}

// jobView resolves one job: which model serves it here, how each of its sites
// scored on that model, and the fold across them.
func jobView(routing ai.RoutingConfig, task ai.Task, sites []aitasks.Site, snap snapshot.Snapshot) crmcontracts.AiCertificationJob {
	job := crmcontracts.AiCertificationJob{Task: string(task), Sites: []crmcontracts.AiCertificationSite{}}

	binding, tier, bound := ai.FirstBoundTier(routing, task)
	if !bound {
		// No rung of this job's ladder has a model. Every site reads the same,
		// and none of them is a statement about the certification lane.
		job.Result = resultNoModel
		for _, site := range sites {
			job.Sites = append(job.Sites, crmcontracts.AiCertificationSite{Site: site.Variant, Result: resultNoModel})
		}
		return job
	}
	job.Provider, job.Model = certPtr(binding.Provider), certPtr(binding.Model)
	job.Tier = certPtr(string(tier))

	env := string(routing.Profile)
	worst := crmcontracts.AiCertificationSite{Result: resultReliable}
	measuredElsewhere, disagreed := false, false
	for _, site := range sites {
		row, found := snap.For(string(task), site.Variant, binding.Provider, binding.Model, env)
		if !found && snap.MeasuredElsewhere(string(task), site.Variant, binding.Provider, binding.Model, env) {
			measuredElsewhere = true
		}
		view := siteView(site.Variant, row, found)
		job.Sites = append(job.Sites, view)
		if len(job.Sites) > 1 && view.Result != job.Sites[0].Result {
			disagreed = true
		}
		if severity[view.Result] >= severity[worst.Result] {
			worst = view
		}
	}
	adoptWorstSite(&job, worst, len(sites), disagreed)
	if measuredElsewhere {
		job.MeasuredUnderOtherProfile = certPtr(true)
	}
	if fallbacks := unmeasuredFallbacks(routing, task, binding, tier, sites, snap); len(fallbacks) > 0 {
		job.UnmeasuredFallbacks = &fallbacks
	}
	return job
}

// adoptWorstSite lifts the weakest site's reading onto the job.
//
// The weakest, never an average: a job is as trustworthy as its least reliable
// part, and averaging four sites would let three sound ones carry one that fails
// every time. worst_site names which one, so the fold stays traceable instead of
// asking a reader to take the verdict on faith.
func adoptWorstSite(job *crmcontracts.AiCertificationJob, worst crmcontracts.AiCertificationSite, siteCount int, disagreed bool) {
	job.Result, job.MeasuredResult = worst.Result, worst.MeasuredResult
	job.Runs, job.Passed = worst.Runs, worst.Passed
	job.RunsPerExample = worst.RunsPerExample
	job.MeasuredExamples, job.PendingExamples = worst.MeasuredExamples, worst.PendingExamples
	job.Scope, job.MeasuredAt = worst.Scope, worst.MeasuredAt
	// Only where the fold actually had to choose. On a job whose sites all read
	// the same — every one reliable, or every one unmeasured — naming one implies
	// it was the reason, and a reader goes looking for a finding that is not
	// there. Ties pick the last site in census order, which is arbitrary.
	if siteCount > 1 && worst.Site != "" && disagreed {
		job.WorstSite = &worst.Site
	}
}

// siteView reads one row into the vocabulary a person can act on.
func siteView(variant string, row snapshot.Row, found bool) crmcontracts.AiCertificationSite {
	view := crmcontracts.AiCertificationSite{Site: variant}
	if !found || row.Status == snapshot.StatusAbsent {
		view.Result = resultNotChecked
		return view
	}
	view.Runs, view.Passed = certPtr(row.Runs), certPtr(row.Passed)
	view.MeasuredExamples, view.PendingExamples = certPtr(row.Measured), certPtr(row.Pending)
	if row.Scope != "" {
		view.Scope = certPtr(row.Scope)
	}
	if at, ok := parseStamp(row.RanAt); ok {
		view.MeasuredAt = &at
	}
	// Derived from this row's own counts rather than quoted from the lane's
	// default: the repeat count is configurable per run, so a figure copied from
	// the runner would be a claim about runs this record never saw.
	if row.Measured > 0 && row.Runs > 0 {
		view.RunsPerExample = certPtr(row.Runs / row.Measured)
	}
	view.Result, view.MeasuredResult = resultOf(row)
	return view
}

// resultOf turns a row's status and band into the word a reader gets, plus —
// where the two differ — what the measurement actually FOUND.
//
// A stale row describes prompts this build no longer sends, and a partial one
// has never seen cases the corpus has since grown. Those are facts about the
// measurement's standing, not about how good the model is, so they become the
// result. But the finding must travel with them: two committed rows are stale
// `not_supported` at 12 of 12 runs passed, and reporting only "Out of date, 12
// of 12" keeps the reassuring half of a measurement whose verdict was that the
// model could not be trusted with the job.
//
// An unrecognized status fails CLOSED. A row spelling a status this build does
// not know is a row this build cannot read, and falling through to the band
// would render it as a reliability finding nobody produced.
func resultOf(row snapshot.Row) (result crmcontracts.AiCertificationResult, measured *crmcontracts.AiCertificationResult) {
	finding := bandResult(row.Band)
	switch row.Status {
	case snapshot.StatusStale:
		return resultOutOfDate, findingIfKnown(finding)
	case snapshot.StatusPartial:
		return resultPartlyChecked, findingIfKnown(finding)
	case snapshot.StatusCurrent:
		return finding, nil
	default:
		return resultNotChecked, nil
	}
}

func bandResult(band string) crmcontracts.AiCertificationResult {
	switch band {
	case "certified":
		return resultReliable
	case "supported_degraded":
		return resultMostlyReliable
	case "not_supported":
		return resultNotReliable
	default:
		// A row with no band measured nothing this site can claim.
		return resultNotChecked
	}
}

// findingIfKnown drops a finding that says nothing. A stale row carrying no
// band has no verdict to report beside its age, and "out of date — it was not
// checked" is noise.
func findingIfKnown(finding crmcontracts.AiCertificationResult) *crmcontracts.AiCertificationResult {
	if finding == resultNotChecked {
		return nil
	}
	return &finding
}

// unmeasuredFallbacks names the models a job can fall back to that nothing has
// graded.
//
// A job's servable set is its ladder plus the transitive degrade closure, and the
// router remaps onto it under budget pressure — so an answer a real deployment
// can reach may come from a model nobody measured. Reported as a caveat rather
// than by demoting the job: the model that answers today is what the reader
// asked about, and calling a sound job unchecked over a rare fallback would
// understate exactly as badly as ignoring the fallback overstates.
func unmeasuredFallbacks(routing ai.RoutingConfig, task ai.Task,
	servingBinding ai.ProviderConfig, serving ai.Tier,
	sites []aitasks.Site, snap snapshot.Snapshot,
) []string {
	env := string(routing.Profile)
	seen := map[string]bool{}
	var out []string
	// servingBinding arrives already resolved from jobView rather than being
	// re-read here: FirstBoundTier fills in the provider default, and a second
	// read of the raw map would compare a resolved model against a document one
	// — which is how a serving ollama rung came to list its own model as an
	// unchecked fallback of itself.
	for _, tier := range ai.ServableTiers(task) {
		if tier == serving {
			continue
		}
		binding := routing.Tiers[tier]
		if !ai.IsBound(binding) {
			continue // an unbound rung serves nothing; it is a routing gap, not a measurement gap
		}
		// Resolved the same way the serving rung was. Without this an ollama
		// degrade target with no model in the document passes IsBound and then
		// names the empty string as an unmeasured fallback.
		binding.Model = ai.EffectiveModel(binding)
		// A lower rung bound to the SAME binding is not a fallback a reader can
		// act on: the row already reports that model, and naming it again as
		// "a fallback we have not checked" contradicts the line above it.
		//
		// Compared as (provider, model), which is how the snapshot identifies a
		// measurement: one model id served through two providers is two bindings
		// with two records, and matching on the id alone would suppress a genuine
		// unmeasured fallback as though it were the serving one.
		if binding.Provider == servingBinding.Provider && binding.Model == servingBinding.Model {
			continue
		}
		identity := binding.Provider + "/" + binding.Model
		if seen[identity] || measuredAnywhere(sites, task, binding, env, snap) {
			continue
		}
		seen[identity] = true
		out = append(out, binding.Model)
	}
	return out
}

// measuredAnywhere says whether this binding has ANY measurement for the job.
//
// Any, not "current everywhere": a fallback measured six weeks ago has been
// graded, and the list this feeds says "which we have not checked". Reporting a
// stale measurement as no measurement is the same defect the row-level
// vocabulary exists to avoid, one level down.
func measuredAnywhere(sites []aitasks.Site, task ai.Task, binding ai.ProviderConfig, env string, snap snapshot.Snapshot) bool {
	for _, site := range sites {
		row, found := snap.For(string(task), site.Variant, binding.Provider, binding.Model, env)
		if found && row.Status != snapshot.StatusAbsent {
			return true
		}
	}
	return false
}

// certPtr takes the address of a value for the contract's optional fields.
//
// Named for this surface rather than `ptr`: package compose is one package, and
// a bare `ptr` here collides with the one handlers_license_test.go already
// declares.
func certPtr[T any](v T) *T { return &v }

// parseStamp reads a record's RFC 3339 timestamp.
//
// A record that carries an unparseable stamp still carries its counts, so the
// row keeps its verdict and loses only the date — dropping the whole
// measurement over a malformed field would hide a real result behind a
// formatting fault.
func parseStamp(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}
