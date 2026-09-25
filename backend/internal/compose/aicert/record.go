// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// Record is one task×provider×model×environment certification outcome —
// the durable, committed artifact a certification run produces and
// `e2e-ai-report` reads back. RanAt is a caller-stamped RFC 3339 timestamp:
// this package never calls time.Now, so the same []RunResult always
// produces the same Record byte-for-byte except for whatever the caller
// puts in RanAt.
type Record struct {
	Task string `json:"task"`
	// Kind is empty on a completion record and KindDecision on a decision
	// record. Kind, Site, Model and Decision are all omitted when empty, so a
	// completion record is written exactly as it was before they existed.
	Kind string `json:"kind,omitempty"`
	// Site is the one site a decision record certifies. A completion record
	// covers every site its scenarios ran on and names them per scenario.
	Site     string `json:"site,omitempty"`
	Provider string `json:"provider"`
	// Model is the CONFIGURED model of a decision record's lane: the runtime
	// looks a certification row up by what it is configured with, never by
	// what a vendor reports having served. ServedModel stays a diagnostic.
	Model         string `json:"model,omitempty"`
	ServedModel   string `json:"served_model"`
	EnvClass      string `json:"env_class"`
	PromptVersion string `json:"prompt_version"`
	CorpusVersion string `json:"corpus_version"`
	Verdict       string `json:"verdict"`
	Runs          int    `json:"runs"`
	// Passed is how many runs did what their scenario asked — the reply the
	// scenario expects, inside its caps — and Reliability is that count over
	// Runs. It is carried as a count of its own because pass/fail is the first
	// question asked of a record, and the four counts below cannot answer it:
	// a run that came back accepted where the scenario demanded an abstention
	// is a FAILED run that still raises ReportedAccepted.
	Passed int `json:"passed"`
	// Reliability is Passed over Runs, and it is the MECHANICAL grade: Evaluate
	// replays the reply through the loop and compares the step taken against
	// the step the scenario expects. A string comparison of tool names, and
	// nothing else — in all three committed agent_loop records passed equals
	// reported_accepted exactly, which is that comparison showing through.
	//
	// THIS is the number the verdict is computed from. JudgeScoreP50 below is
	// louder and is not, which is the confusion #881 was filed for: the two
	// moved +0.019 and +65 on the same scenarios, the same binding and the same
	// judge, because they are not grading the same thing.
	Reliability float64 `json:"reliability"`
	// ReportedAccepted/ReportedWrongAnswer/ReportedInvalid/ReportedAbstained
	// are what the sites' own validators REPORTED across the pooled runs, one
	// count per outcome — never a pass/fail column. They are what turns a
	// reliability number into a diagnosis: replies the validator refused want a
	// different fix from well-formed replies that say the wrong thing, and an
	// abstention is a right answer that neither of the other two can express.
	// They always sum to Runs.
	ReportedAccepted    int `json:"reported_accepted"`
	ReportedWrongAnswer int `json:"reported_wrong_answer"`
	ReportedInvalid     int `json:"reported_invalid"`
	ReportedAbstained   int `json:"reported_abstained"`
	// CertifiedScope is how much of the task this record actually covers (one of
	// the aitasks.Scope* words), folded to the NARROWEST scope any site the run
	// touched could claim. A multi-turn or agent-loop scenario seeds the window
	// and grades the one reply that follows; a case bound to a site that calls
	// the model more than once for one invocation grades one of those calls. In
	// both, something the product does is supplied or skipped rather than
	// exercised, and a record silent about it claims more than it tested.
	CertifiedScope string `json:"certified_scope"`
	// ContextApplied says whether the runs were served the company context
	// production prepends. It is recorded rather than implied because the
	// answer is no: assembling that context reads the database, and the cert
	// lane runs without one. A record that omitted the field would leave a
	// reader to assume parity nobody checked.
	ContextApplied bool `json:"context_applied"`
	// ContextScopes is what THIS task's contract has production prepend, and it
	// is what makes ContextApplied's answer readable on one row instead of only
	// as a fact about the lane. Most tasks declare no scopes and lose nothing by
	// running DB-less; the ones that declare some were certified without
	// reference data every production call carries, and how much that costs
	// starts with which scopes they are.
	//
	// It is read off the task contract at build time rather than kept as a list
	// beside it, so a scope added to a task cannot leave a record naming the old
	// set.
	ContextScopes []string `json:"context_scopes"`
	// JudgeScoreP50/JudgeScoreMin are the JUDGE's grade: a model scoring the
	// reply's prose against a written rubric, pooled across the runs.
	//
	// Named for their grader because the old spelling did not say who produced
	// them, and a reader comparing them with Reliability above was comparing an
	// opinion of the writing with a verdict on the behaviour. The judge can like
	// a reply that took the wrong step and dislike one that took the right step;
	// neither grader is wrong on its own terms, and only Reliability governs the
	// verdict.
	//
	// Kept rather than removed, because for the drafting tasks prose quality IS
	// what is being certified and this is the useful number there. What was
	// wrong was only that on agent_loop it sat beside a mechanical verdict with
	// nothing saying which was which.
	JudgeScoreP50 int   `json:"judge_score_p50"`
	JudgeScoreMin int   `json:"judge_score_min"`
	LatencyP50    int64 `json:"latency_p50"`
	LatencyP95    int64 `json:"latency_p95"`
	MeanTokens    int   `json:"mean_tokens"`
	// MeanTokensIn/MeanTokensOut/MeanCachedTokens/MeanCacheWriteTokens are
	// the four-bucket baseline (ADR-0067 phase 2): the pooled run set's
	// per-bucket mean, each bucket's own truncating integer division —
	// independent of MeanTokens (kept for compat), which divides the exact
	// summed total instead, so the two need not add up bucket-for-bucket.
	MeanTokensIn         int    `json:"mean_tokens_in"`
	MeanTokensOut        int    `json:"mean_tokens_out"`
	MeanCachedTokens     int    `json:"mean_cached_tokens"`
	MeanCacheWriteTokens int    `json:"mean_cache_write_tokens"`
	EstCostMicroUSD      int64  `json:"est_cost_microusd"`
	JudgeServedModel     string `json:"judge_served_model"`
	SelfJudged           bool   `json:"self_judged"`
	ServedIdentitySource string `json:"served_identity_source"`
	// CandidateUpstream and JudgeUpstream are the broker upstream preferences
	// each binding was served under, absent for a binding none reach. They
	// change which hosts may answer — the precision, the ceiling, the tail — so
	// two records of one model are comparable only where these agree; a broker
	// record without them predates the product default being applied here.
	CandidateUpstream *ai.OpenRouterRouting `json:"candidate_upstream,omitempty"`
	JudgeUpstream     *ai.OpenRouterRouting `json:"judge_upstream,omitempty"`
	RanAt             string                `json:"ran_at"`
	// Decision is what the decision lane did across a decision record's runs.
	Decision *DecisionStats `json:"decision,omitempty"`
	// Scenarios is every scenario this record pooled, with its own verdict and
	// its own counts. A record is written per TASK and a task is not one
	// scenario or even one site — cold_start ships four sites — so the pooled
	// numbers above cannot answer the question a failure actually raises:
	// WHICH scenario failed. One injection scenario failing every run and one
	// ordinary scenario failing occasionally reach the same task reliability
	// and want completely different fixes.
	Scenarios []ScenarioRecord `json:"scenarios"`
}

// ScenarioRecord is one scenario's own share of a task's record: what it asked
// for, how often it got it, and what came back the rest of the time.
//
// Site is carried per scenario rather than derived, because it is what lets a
// reader — and the readiness report — attribute a task's record to the site
// each of its rows actually measured.
type ScenarioRecord struct {
	Scenario string `json:"scenario"`
	Site     string `json:"site"`
	// Stamp is THIS scenario's own certification stamp — the scenario whole plus
	// the candidate and grader requests this build constructs from it
	// (ScenarioStamps). The record's task-level PromptVersion is the fold of
	// every one of these.
	//
	// Carried per scenario because the fold cannot say WHICH scenario moved, and
	// that is the difference between re-certifying one case and re-certifying a
	// task: adding a tenth scenario leaves nine measurements true, and a task
	// stamp has no way to say so. Empty on a record written before this field
	// existed, which the report reads as "ask the task stamp instead".
	Stamp   string `json:"stamp,omitempty"`
	Verdict string `json:"verdict"`
	// JudgeBand is the best verdict this scenario's judge scores alone reach
	// against its own bands — what a verdict over several rows needs of each.
	JudgeBand string `json:"judge_band,omitempty"`
	Runs      int    `json:"runs"`
	Passed    int    `json:"passed"`
	// The same reported-outcome counts the task carries, on this scenario's own
	// runs: they say what came back, never whether it was what was asked for.
	ReportedAccepted    int `json:"reported_accepted"`
	ReportedWrongAnswer int `json:"reported_wrong_answer"`
	ReportedInvalid     int `json:"reported_invalid"`
	ReportedAbstained   int `json:"reported_abstained"`
	// Withheld is how many of these runs the provider withheld an answer from,
	// and WithheldReasons the distinct filters it named, so a reader can tell a
	// safety stop from a model that answered badly.
	Withheld        int      `json:"withheld,omitempty"`
	WithheldReasons []string `json:"withheld_reasons,omitempty"`
	// Decision is this scenario's own share of a decision record.
	Decision *DecisionStats `json:"decision,omitempty"`
}

// KindDecision marks a record of the decision lane: one site, one configured
// lane model, graded by the site's own gate and nothing else.
const KindDecision = "decision"

// DecisionStats is what the decision lane did across a set of runs. A run is
// KEPT when the site's own gate accepted the answer, and it FELL BACK
// otherwise; only a kept answer can be wrong, because a fallback hands the
// question to the LLM ladder, which its own record measures.
type DecisionStats struct {
	Kept        int `json:"kept"`
	KeptCorrect int `json:"kept_correct"`
	// KeptWrong is the number the verdict turns on: an answer the site would
	// have acted on, and was wrong.
	KeptWrong    int     `json:"kept_wrong"`
	Fallbacks    int     `json:"fallbacks"`
	FallbackRate float64 `json:"fallback_rate"`
	// FallbackByReason counts the fallbacks by the attempt reason the ladder
	// walk would carry, without its "decision_" prefix: below_floor, error,
	// off_enum, state_too_large, local_only.
	FallbackByReason map[string]int `json:"fallback_by_reason,omitempty"`
	// MinKeptConfidence is the least confident answer the gate kept. A
	// diagnostic of how close the floor is, never a floor itself.
	MinKeptConfidence float64 `json:"min_kept_confidence"`
	// ServedPassRate is what a caller of the site gets: kept-correct answers,
	// plus the LLM record's pass rate on the runs that fell back to it.
	ServedPassRate float64 `json:"served_pass_rate"`
}

// SiteTally is one SITE's share of a task's record, folded from the scenario
// rows that ran on it.
//
// It exists because a record covers a task and a reader asks about a site: a
// task's pooled numbers printed under four site labels are the same four
// numbers wearing labels they did not earn.
type SiteTally struct {
	Verdict             string
	Runs                int
	Passed              int
	ReportedAccepted    int
	ReportedWrongAnswer int
	ReportedInvalid     int
	ReportedAbstained   int
}

// Reliability is the fraction of this site's runs that did what their scenario
// asked. A site with no run has no reliability rather than a perfect one.
func (t SiteTally) Reliability() float64 {
	if t.Runs == 0 {
		return 0
	}
	return float64(t.Passed) / float64(t.Runs)
}

// ForSite folds every scenario row this record kept for one site's variant.
// False means this record measured that site not at all — a different thing
// from measuring it and finding nothing, which is why it is not a zero tally.
//
// The verdict is Verdict's rule over the site's scenario rows, the same rule
// the task's own verdict is.
func (r Record) ForSite(variant string) (SiteTally, bool) {
	var tally SiteTally
	var rows []scenarioTally
	for _, sc := range r.Scenarios {
		if sc.Site != variant {
			continue
		}
		// A row written before JudgeBand existed: its own verdict is the most its
		// scores are known to reach, which reproduces the worst-scenario fold.
		band := sc.JudgeBand
		if band == "" {
			band = sc.Verdict
		}
		rows = append(rows, scenarioTally{runs: sc.Runs, passed: sc.Passed, judgeBand: band})
		tally.Runs += sc.Runs
		tally.Passed += sc.Passed
		tally.ReportedAccepted += sc.ReportedAccepted
		tally.ReportedWrongAnswer += sc.ReportedWrongAnswer
		tally.ReportedInvalid += sc.ReportedInvalid
		tally.ReportedAbstained += sc.ReportedAbstained
	}
	if len(rows) == 0 {
		return SiteTally{}, false
	}
	tally.Verdict, _ = verdictOver(rows)
	return tally, true
}

// sanitizeForPath maps a raw identifier (a provider name, or a served-model
// string like "accounts/fireworks/models/llama-v3-70b-instruct" that
// carries filesystem-hostile characters) to a safe path segment: every "/"
// and ":" becomes "_". This is a one-way, lossy mapping — two distinct raw
// strings could collide on the same sanitized segment — but it is
// deterministic, which is the property WriteRecord/LoadRecords actually
// need: the same raw string always resolves to the same file.
func sanitizeForPath(s string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	return replacer.Replace(s)
}

// recordPath returns the file WriteRecord/LoadRecords use for r under dir:
// records/<task>/<provider>_<model>_<env>.json, and for a decision record
// records/<task>/decision_<site>_<provider>_<model>_<env>.json under the
// configured model, which is the one its certification row is keyed by.
func recordPath(dir string, r Record) string {
	filename := fmt.Sprintf("%s_%s_%s.json",
		sanitizeForPath(r.Provider), sanitizeForPath(r.ServedModel), sanitizeForPath(r.EnvClass))
	if r.Kind == KindDecision {
		filename = fmt.Sprintf("decision_%s_%s_%s_%s.json", sanitizeForPath(r.Site),
			sanitizeForPath(r.Provider), sanitizeForPath(r.Model), sanitizeForPath(r.EnvClass))
	}
	return filepath.Join(dir, sanitizeForPath(r.Task), filename)
}

// WriteRecord persists r under dir at its task/provider/model/env path,
// creating parent directories as needed. Marshaling is stable — fixed
// struct field order via json.MarshalIndent, a trailing newline — so a
// re-run that produces an identical Record leaves a diff-free file; only a
// genuine change in outcome touches the committed record.
func WriteRecord(dir string, r Record) error {
	path := recordPath(dir, r)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("aicert: creating %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("aicert: marshaling record for %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("aicert: writing %s: %w", path, err)
	}
	return nil
}

// LoadRecords reads every *.json file under dir into a Record, sorted by
// Task/Provider/ServedModel/EnvClass so a report over the same record set
// always renders the same order. A directory that does not exist yet
// (no certification has run) is not an error — it reads as an empty
// record set, the honest "nothing certified yet" state.
func LoadRecords(dir string) ([]Record, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("aicert: records %s: %w", dir, err)
	}

	var records []Record
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("aicert: records %s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, readErr := os.ReadFile(path) // #nosec G304 G122 -- path is a *.json file from walking the trusted records tree
		if readErr != nil {
			return fmt.Errorf("aicert: reading %s: %w", path, readErr)
		}
		var r Record
		if decodeErr := json.Unmarshal(raw, &r); decodeErr != nil {
			return fmt.Errorf("aicert: parsing %s: %w", path, decodeErr)
		}
		// A file under this tree that names no task is not a certification
		// record. The use-case lane files its verdicts here too — one folder per
		// model under UseCaseVerdictDir — because they belong beside the records
		// they sit next to in a reader's mind. They carry no task and no
		// binding, so admitting them would publish a row with an empty identity
		// and count it in a total that says how much has been certified.
		if r.Task == "" {
			return nil
		}
		records = append(records, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.Task != b.Task {
			return a.Task < b.Task
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.ServedModel != b.ServedModel {
			return a.ServedModel < b.ServedModel
		}
		if a.EnvClass != b.EnvClass {
			return a.EnvClass < b.EnvClass
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Site < b.Site
	})
	return records, nil
}
