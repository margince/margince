// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// RestampBasis holds what a re-stamp compares a record's rows against: each
// scenario's stamp under this build's grading rule, and under the rule the
// record was graded by. Both are keyed "task/scenario", the name a stamp map is
// unique by.
type RestampBasis struct {
	now, was map[string]string
}

// NewRestampBasis stamps corpus under this build's rule and under oldRule. The
// two differ in the grader third alone when only the rule moved, which is what
// Restamp reads off a row.
func NewRestampBasis(ctx context.Context, oldRule string, corpus []Scenario, census *aitasks.Registry) (RestampBasis, error) {
	basis := RestampBasis{now: map[string]string{}, was: map[string]string{}}
	byTask := map[string][]Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	for task, scenarios := range byTask {
		for rule, into := range map[string]map[string]string{gradingRule: basis.now, oldRule: basis.was} {
			stamps, err := scenarioStampsUnder(ctx, rule, scenarios, census)
			if err != nil {
				return RestampBasis{}, fmt.Errorf("task %s: %w", task, err)
			}
			for name, stamp := range stamps {
				into[task+"/"+name] = stamp
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

// Restamp grades rec again from its stored rows under this build's rule, when
// only the rule moved since rec was graded by oldRule. A rule change alters how
// stored outcomes pool and grade, never what a model was asked or how one run
// was checked, so the rows are still true. Anything else is refused with the
// reason, and the record stays stale for a paid run.
//
// A row proves only the rule moved when its case and prompt thirds equal
// today's and its grader third equals what oldRule would stamp today. The
// second half is what rules out a site check or a judge request that moved
// with the rule: either would put a different digest under oldRule too.
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
	regradeFromRows(&out)
	return Restamped{Record: out, Changed: true}
}

// restampRefusal names why a record cannot be re-stamped whatever its stamps
// say, or answers "".
func restampRefusal(rec Record) string {
	switch {
	case rec.Kind != "":
		return "a decision record is stamped by its own rule"
	case gradedElsewhere(rec):
		return fmt.Sprintf("graded by judge %s, and this build grades with %s", rec.JudgeServedModel, DefaultJudgeModel)
	case rec.WireRule != ai.WireRule(rec.Provider):
		return fmt.Sprintf("measured on wire rule %q, and this build sends %q", rec.WireRule, ai.WireRule(rec.Provider))
	case len(rec.Scenarios) == 0:
		return "it holds no scenario rows"
	}
	return ""
}
