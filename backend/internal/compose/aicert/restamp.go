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
	basis := RestampBasis{now: map[string]string{}, was: map[string]string{}}
	byTask := map[string][]Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	for task, scenarios := range byTask {
		for _, pass := range []struct {
			rule string
			into map[string]string
		}{{gradingRule, basis.now}, {oldRule, basis.was}} {
			stamps, err := scenarioStampsUnder(ctx, pass.rule, scenarios, census)
			if err != nil {
				return RestampBasis{}, fmt.Errorf("task %s: %w", task, err)
			}
			for name, stamp := range stamps {
				pass.into[task+"/"+name] = stamp
			}
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
				return Restamped{Record: rec, Refusal: fmt.Sprintf("scenario %q: its site check or judge request moved with the rule", row.Scenario)}
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
