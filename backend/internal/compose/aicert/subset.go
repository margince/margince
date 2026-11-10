// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Measuring part of a task. SITE= and SCENARIO= narrow a run to some of a
// task's scenarios. The rows they measure replace their own in the committed
// record, instead of a record of the subset alone.
//
// A record of a subset would read as the task's verdict on a fraction of its
// cases. Each row keeps its own stamp, so the readiness report still tells a
// current row from a stale one after the merge.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// scenarioSubset is the part of each task a filtered run measures. The zero
// value keeps every scenario.
type scenarioSubset struct {
	site string
	name *regexp.Regexp
}

// subsetOf compiles the run's filters. An invalid pattern is refused before
// any call is paid for.
func subsetOf(cfg RunnerConfig) (scenarioSubset, error) {
	subset := scenarioSubset{site: cfg.SiteFilter}
	if cfg.ScenarioFilter != "" {
		name, err := regexp.Compile(cfg.ScenarioFilter)
		if err != nil {
			return scenarioSubset{}, fmt.Errorf("MARGINCE_AICERT_SCENARIO=%q is not a regular expression: %w", cfg.ScenarioFilter, err)
		}
		subset.name = name
	}
	return subset, nil
}

// narrows says the run measures only part of some task.
func (s scenarioSubset) narrows() bool { return s.site != "" || s.name != nil }

func (s scenarioSubset) keeps(sc Scenario) bool {
	return (s.site == "" || sc.Site == s.site) && (s.name == nil || s.name.MatchString(sc.Name))
}

// apply narrows each task to the scenarios the subset keeps and drops a task
// left with none. A filter that keeps nothing at all is refused, since the run
// would measure nothing.
func (s scenarioSubset) apply(byTask map[ai.Task][]Scenario) (map[ai.Task][]Scenario, error) {
	if !s.narrows() {
		return byTask, nil
	}
	narrowed := map[ai.Task][]Scenario{}
	for task, scenarios := range byTask {
		kept := slices.DeleteFunc(slices.Clone(scenarios), func(sc Scenario) bool { return !s.keeps(sc) })
		if len(kept) > 0 {
			narrowed[task] = kept
		}
	}
	if len(narrowed) == 0 {
		return nil, errors.New("MARGINCE_AICERT_SITE / MARGINCE_AICERT_SCENARIO match no scenario of the tasks this run certifies")
	}
	return narrowed, nil
}

// mergeIntoCommitted folds a filtered run's record into the record already
// committed for the same candidate. Its rows replace theirs, every other row
// still in corpus is kept, and the task's grade is recomputed from all of them.
//
// It refuses when the committed record cannot be graded again from its rows,
// or was measured under another binding or judge. A merge would then pool two
// measurements under one heading.
//
// The pooled latency, token and cost numbers stay the committed record's. Rows
// carry none of them, and the measured subset alone is a biased sample of the task.
func mergeIntoCommitted(dir string, measured Record, corpus []Scenario) (Record, error) {
	path := recordPath(dir, measured)
	raw, err := os.ReadFile(path) // #nosec G304 -- recordPath under the operator's record directory
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, fmt.Errorf("SITE=/SCENARIO= measured %d scenario(s), and no record at %s holds the rest of the task; "+
			"certify the task without a filter first", len(measured.Scenarios), path)
	}
	if err != nil {
		return Record{}, fmt.Errorf("reading the record to merge into: %w", err)
	}
	var prior Record
	if err := json.Unmarshal(raw, &prior); err != nil {
		return Record{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := mergeable(prior, measured); err != nil {
		return Record{}, fmt.Errorf("%s cannot take a filtered run's rows: %w; certify the task without a filter", path, err)
	}
	rows, err := mergedRows(prior.Scenarios, measured.Scenarios, corpus)
	if err != nil {
		return Record{}, fmt.Errorf("%s cannot take a filtered run's rows: %w", path, err)
	}
	merged := measured
	merged.Scenarios = rows
	regradeFromRows(&merged)
	merged.CertifiedScope = aitasks.NarrowerScope(prior.CertifiedScope, measured.CertifiedScope)
	merged.ContextApplied = prior.ContextApplied && measured.ContextApplied
	// Every row the record pools vouches for these, so each reads the worse run:
	// a judge of the candidate's family, and the oldest measurement.
	merged.SelfJudged = prior.SelfJudged || measured.SelfJudged
	merged.RanAt = olderRanAt(prior.RanAt, measured.RanAt)
	merged.LatencyP50, merged.LatencyP95 = prior.LatencyP50, prior.LatencyP95
	merged.MeanTokens, merged.MeanTokensIn, merged.MeanTokensOut = prior.MeanTokens, prior.MeanTokensIn, prior.MeanTokensOut
	merged.MeanCachedTokens, merged.MeanCacheWriteTokens = prior.MeanCachedTokens, prior.MeanCacheWriteTokens
	merged.EstCostMicroUSD = prior.EstCostMicroUSD
	return merged, nil
}

// mergeable refuses a committed record whose rows were measured under
// something the new rows were not.
func mergeable(prior, measured Record) error {
	switch {
	case prior.Kind != measured.Kind || prior.CorpusVersion != measured.CorpusVersion:
		return errors.New("it is another kind of record or corpus version")
	case prior.ThinkingLevel != measured.ThinkingLevel || !reflect.DeepEqual(prior.SiteThinking, measured.SiteThinking):
		return errors.New("its runs were measured at another thinking level")
	case !reflect.DeepEqual(prior.CandidateUpstream, measured.CandidateUpstream) ||
		!reflect.DeepEqual(prior.JudgeUpstream, measured.JudgeUpstream):
		return errors.New("its runs were served under other upstream preferences")
	case prior.JudgeProvider != measured.JudgeProvider || prior.JudgeServedModel != measured.JudgeServedModel:
		return errors.New("another judge graded it")
	case prior.ServedIdentitySource != measured.ServedIdentitySource:
		return errors.New("its served model was identified another way")
	}
	return nil
}

// olderRanAt is the earlier of two record timestamps, the committed one when
// either cannot be read.
func olderRanAt(prior, measured string) string {
	priorAt, perr := time.Parse(time.RFC3339, prior)
	measuredAt, merr := time.Parse(time.RFC3339, measured)
	if perr != nil || merr != nil || priorAt.Before(measuredAt) {
		return prior
	}
	return measured
}

// mergedRows is the measured rows plus every committed row they do not
// replace, in corpus order. A row whose scenario left the corpus is dropped,
// as a full run would drop it. A corpus scenario with no row is refused, since
// a full run would have measured it.
func mergedRows(prior, measured []ScenarioRecord, corpus []Scenario) ([]ScenarioRecord, error) {
	order := map[string]int{}
	for i, sc := range corpus {
		order[sc.Name] = i
	}
	rows := slices.Clone(measured)
	for _, row := range prior {
		_, inCorpus := order[row.Scenario]
		replaced := slices.ContainsFunc(measured, func(m ScenarioRecord) bool { return m.Scenario == row.Scenario })
		if !inCorpus || replaced {
			continue
		}
		if row.legacy() || row.Stamp == "" {
			return nil, fmt.Errorf("row %q predates rows that keep their stamp, scores and bands; certify the task without a filter", row.Scenario)
		}
		rows = append(rows, row)
	}
	var missing []string
	for _, sc := range corpus {
		if !slices.ContainsFunc(rows, func(r ScenarioRecord) bool { return r.Scenario == sc.Name }) {
			missing = append(missing, sc.Name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("scenario(s) %s have no row; measure them too, by naming them in SCENARIO=",
			strings.Join(missing, ", "))
	}
	slices.SortStableFunc(rows, func(a, b ScenarioRecord) int { return order[a.Scenario] - order[b.Scenario] })
	return rows, nil
}

// regradeFromRows recomputes every number of rec its rows determine: the
// verdict, the counts, the judge's pooled grade, and the task stamp.
func regradeFromRows(rec *Record) {
	cases := make([]caseStats, 0, len(rec.Scenarios))
	stamps := map[string]string{}
	var scored []RunResult
	rec.Runs, rec.Passed = 0, 0
	rec.ReportedAccepted, rec.ReportedWrongAnswer, rec.ReportedInvalid, rec.ReportedAbstained = 0, 0, 0, 0
	for _, row := range rec.Scenarios {
		cases = append(cases, rowCase(row))
		stamps[row.Scenario] = row.Stamp
		for _, score := range row.JudgeScores {
			scored = append(scored, RunResult{Score: score})
		}
		rec.Runs += row.Runs
		rec.Passed += row.Passed
		rec.ReportedAccepted += row.ReportedAccepted
		rec.ReportedWrongAnswer += row.ReportedWrongAnswer
		rec.ReportedInvalid += row.ReportedInvalid
		rec.ReportedAbstained += row.ReportedAbstained
	}
	rec.Verdict, rec.Reliability = verdictOver(cases)
	rec.JudgeScoreP50, rec.JudgeScoreMin, _ = judgeMedianAndMin(scored)
	rec.PromptVersion = FoldScenarioStamps(stamps)
}
