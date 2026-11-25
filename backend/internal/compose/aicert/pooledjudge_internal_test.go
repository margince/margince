// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestATaskNoJudgeScoredHasNoPooledJudgeScore(t *testing.T) {
	ungraded := []RunResult{{Ungraded: true}, {Ungraded: true}}
	p50, minimum := pooledJudgeScores(ungraded)
	if p50 != nil || minimum != nil {
		t.Fatalf("pooled scores of runs no judge graded = %v, %v; want none", p50, minimum)
	}
	if got := JudgeScoreCell(p50); got != "not judged" {
		t.Errorf("cell = %q, want %q", got, "not judged")
	}
	body, err := json.Marshal(Record{JudgeScoreP50: p50, JudgeScoreMin: minimum})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "judge_score") {
		t.Errorf("a record no judge scored still carries a pooled score: %s", body)
	}
}

func TestAJudgedTaskPoolsItsScoresAndKeepsAGradedZero(t *testing.T) {
	p50, minimum := pooledJudgeScores([]RunResult{{Score: 0}, {Score: 60}, {Ungraded: true}})
	if p50 == nil || minimum == nil || *minimum != 0 || *p50 != 30 {
		t.Fatalf("pooled = %v, %v; want median 30 and minimum 0", p50, minimum)
	}
	if got := JudgeScoreCell(minimum); got != "0" {
		t.Errorf("a judge that scored 0 reads %q, want 0", got)
	}
}
