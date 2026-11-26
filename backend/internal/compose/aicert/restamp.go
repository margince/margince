// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// restampableRules maps a rule to the one that replaced it. The change moved how
// stored rows pool and grade, and nothing about how scores are collected. A
// reviewer who read that change adds the pair. Records of any other pair need
// new runs, because their rows are not what this rule would record.
var restampableRules = map[string]string{"grading-rule-13": "grading-rule-14"}

// collectionOfRule is the collectionRule each grading rule ran under. A record
// re-stamps only from a rule whose collection is today's. A moved collection
// changes the rows a build would record, and no re-grade of stored rows can
// reproduce that. Every bump of gradingRule adds its entry.
var collectionOfRule = map[string]string{"grading-rule-13": "collection-rule-1", "grading-rule-14": "collection-rule-1"}

// RestampBasis holds what a re-stamp compares a row against, keyed
// "task/scenario". One stamp is under this build's grading rule. The other is
// under the rule the record was graded by.
type RestampBasis struct {
	now, was map[string]string
}

// NewRestampBasis stamps corpus under this build's rule and under oldRule. When
// only the rule moved, the two differ in the grader third alone.
func NewRestampBasis(ctx context.Context, oldRule string, corpus []Scenario, census *aitasks.Registry) (RestampBasis, error) {
	if restampableRules[oldRule] != gradingRule {
		return RestampBasis{}, fmt.Errorf("aicert: restamp: no review says %q moved only how rows pool and grade into %q, "+
			"so its records need new runs; add the pair to restampableRules once a reviewer has checked that", oldRule, gradingRule)
	}
	if was := collectionOfRule[oldRule]; was != collectionRule {
		return RestampBasis{}, fmt.Errorf("aicert: restamp: %q collected its runs under %q and this build under %q, "+
			"so its stored rows are not what this build would record and its records need new runs", oldRule, was, collectionRule)
	}
	return restampBasisBetween(ctx, corpus, census, func(scenarios []Scenario) (map[string]string, error) {
		return scenarioStampsUnder(ctx, oldRule, scenarios, census)
	})
}

// restampBasisBetween stamps corpus as this build stamps it and as was stamps
// it, task by task.
func restampBasisBetween(
	ctx context.Context, corpus []Scenario, census *aitasks.Registry, was func([]Scenario) (map[string]string, error),
) (RestampBasis, error) {
	basis := RestampBasis{now: map[string]string{}, was: map[string]string{}}
	byTask := map[string][]Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	for task, scenarios := range byTask {
		now, err := scenarioStampsUnder(ctx, gradingRule, scenarios, census)
		if err != nil {
			return RestampBasis{}, fmt.Errorf("task %s: %w", task, err)
		}
		before, err := was(scenarios)
		if err != nil {
			return RestampBasis{}, fmt.Errorf("task %s: %w", task, err)
		}
		for name, stamp := range now {
			basis.now[task+"/"+name] = stamp
		}
		for name, stamp := range before {
			basis.was[task+"/"+name] = stamp
		}
	}
	return basis, nil
}

// Restamped is what Restamp says of one record. Refusal is empty when the record
// was re-stamped or was already current; Record is then the one to write.
type Restamped struct {
	Record  Record
	Changed bool
	Refusal string
}

// Restamp grades rec again from its stored rows when only the rule moved since
// oldRule graded it, and refuses with the reason otherwise.
//
// A row proves that when its case and prompt thirds equal today's and its grader
// third equals what oldRule stamps today. A moved site check or judge request
// would change that digest too. A record this rule would extend is refused,
// because its rows are not the ones the rule would have kept.
func (b RestampBasis) Restamp(rec Record) Restamped {
	if refusal := restampRefusal(rec); refusal != "" {
		return Restamped{Record: rec, Refusal: refusal}
	}
	out := rec
	out.Scenarios = make([]ScenarioRecord, len(rec.Scenarios))
	changed := false
	for i, row := range rec.Scenarios {
		key := rec.Task + "/" + row.Scenario
		now, was := b.now[key], b.was[key]
		if now == "" {
			return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q is no longer in the corpus", row.Scenario)}
		}
		if row.legacy() {
			return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q predates stored bands", row.Scenario)}
		}
		if len(row.Stamp) != stampSegment*3 {
			return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q carries no per-scenario stamp", row.Scenario)}
		}
		if row.Stamp != now {
			if moved := splitStampChange(row.Stamp, now); moved.Case || moved.Prompt {
				return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q: %s moved, so it needs new runs", row.Scenario, moved.Describe())}
			}
			if row.Stamp != was {
				return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q: its site check or judge request is not the one the earlier basis stamps", row.Scenario)}
			}
			changed = true
		}
		row.Stamp = now
		out.Scenarios[i] = row
	}
	if !changed {
		return Restamped{Record: rec}
	}
	if name := wantsMoreRuns(out.Scenarios); name != "" {
		return Restamped{Record: rec, Refusal: fmt.Sprintf("this rule would run scenario %q again", name)}
	}
	regradeFromRows(&out)
	return Restamped{Record: out, Changed: true}
}

// restampRefusal names why a record cannot be re-stamped whatever its stamps
// say, or answers "".
func restampRefusal(rec Record) string {
	switch {
	case rec.Kind != "":
		return "a decision record is stamped by its own rule"
	case rec.JudgeServedModel == "" && slices.ContainsFunc(rec.Scenarios, func(row ScenarioRecord) bool { return !row.JudgeNone }):
		return "a judged record names no judge, so it cannot be shown to have been graded by the default one"
	case gradedElsewhere(rec):
		return fmt.Sprintf("graded by judge %s, and this build grades with %s", rec.JudgeServedModel, DefaultJudgeModel)
	case rec.WireRule != ai.WireRule(rec.Provider):
		return fmt.Sprintf("measured on wire rule %q, and this build sends %q", rec.WireRule, ai.WireRule(rec.Provider))
	case len(rec.Scenarios) == 0:
		return "it holds no scenario rows"
	}
	return ""
}

// wantsMoreRuns names a scenario that the adaptive rounds of this rule would
// extend, as nextRunCounts would, or answers "".
func wantsMoreRuns(rows []ScenarioRecord) string {
	cases := make([]caseStats, len(rows))
	for i, row := range rows {
		cases[i] = rowCase(row)
	}
	pooled := poolUndecided(cases)
	for i, row := range rows {
		if extendsCase(cases[i], pooled, adaptiveMaxRuns) {
			return row.Scenario
		}
	}
	return ""
}

// RestampAll re-stamps each completion record of records. It answers one line
// per record it changed or refused, and the records to write. A record already
// current and a decision record, which has its own stamp, say nothing.
func (b RestampBasis) RestampAll(records []Record) (report []string, changed []Record) {
	for _, rec := range records {
		if rec.Kind == KindDecision {
			continue
		}
		got := b.Restamp(rec)
		switch {
		case got.Refusal != "":
			report = append(report, fmt.Sprintf("stale    %s: %s", RecordKey(rec), got.Refusal))
		case got.Changed:
			report = append(report, fmt.Sprintf("restamp  %s: %s -> %s", RecordKey(rec), rec.Verdict, got.Record.Verdict))
			changed = append(changed, got.Record)
		}
	}
	return report, changed
}
