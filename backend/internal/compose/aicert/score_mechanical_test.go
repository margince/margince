// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// mechanical is one judge-less case of nine runs, the first passes passing.
// It carries bands and a 0 a judge never gave, so a rule that read either
// would pool a margin of −70 per run.
func mechanical(passes int) aicert.ScenarioRuns {
	set := aicert.ScenarioRuns{Mechanical: true, Bands: testBands, Runs: make([]aicert.RunResult, 9)}
	for i := range passes {
		set.Runs[i].HardPass = true
	}
	return set
}

// A case declaring judge: none meets the judge half by construction, so its
// pass count is the whole of what it contributes: it certifies a task on its
// mechanics, stays out of the judged cases' margins, and vetoes as any case does.
func TestAJudgelessCaseIsGradedOnItsPassCountAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		sets []aicert.ScenarioRuns
		want string
	}{
		{"a task of judge-less cases certifies on mechanics alone", repeated(4, mechanical(9)), aicert.VerdictCertified},
		{
			"a judge-less case adds no margin to the judged cases' pool",
			[]aicert.ScenarioRuns{graded(testBands, 9, 76, 78, 80, 76, 78, 80, 76, 78, 80), mechanical(9), mechanical(9)},
			aicert.VerdictCertified,
		},
		{
			"a judged case below its bar still holds a mixed task back",
			[]aicert.ScenarioRuns{graded(testBands, 9, 60, 60, 60, 60, 60, 60, 60, 60, 60), mechanical(9)},
			aicert.VerdictSupportedDegraded,
		},
		{"a judge-less case failing every run vetoes the task", append(repeated(3, mechanical(9)), mechanical(0)), aicert.VerdictNotSupported},
		{"a judge-less case passing under half blocks certified", append(repeated(9, mechanical(9)), mechanical(3)), aicert.VerdictSupportedDegraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := aicert.Verdict(tc.sets...); got != tc.want {
				t.Errorf("verdict = %q, want %q", got, tc.want)
			}
		})
	}
}

// A judge-less row has no bands and no scores, and is regraded from its counts
// rather than read as a row that predates the rule.
func TestAJudgelessRowIsRegradedFromItsCounts(t *testing.T) {
	row := aicert.ScenarioRecord{Site: "acts", Runs: 9, Passed: 0, JudgeNone: true, Verdict: aicert.VerdictCertified}
	if got := row.CaseVerdict(); got != aicert.VerdictNotSupported {
		t.Errorf("a judge-less row failing every run reads %q, want not_supported", got)
	}
	row.Passed = 9
	if got := row.CaseJudgeBand(); got != aicert.VerdictCertified {
		t.Errorf("a judge-less row's judge half reads %q, want certified by construction", got)
	}
	rec := aicert.Record{Verdict: aicert.VerdictNotSupported, Scenarios: []aicert.ScenarioRecord{row, row}}
	if tally, _ := rec.ForSite("acts"); tally.Verdict != aicert.VerdictCertified {
		t.Errorf("site verdict = %q, want certified — regraded from its rows, not the stored verdict", tally.Verdict)
	}
}
