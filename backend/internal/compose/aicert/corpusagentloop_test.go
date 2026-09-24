// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// agentLoopFirstStepOnly is the phrase an agent_loop rubric uses to tell the judge
// what it is shown.
const agentLoopFirstStepOnly = "FIRST step only"

// An agent_loop case grades one step of a multi-step turn, so a rubric that does
// not say so lets the judge mark a right first call down for the steps it never saw.
func TestEveryAgentLoopRubricGradesTheFirstStepOnly(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("LoadCorpus(corpus): %v", err)
	}
	found := 0
	for _, sc := range scenarios {
		if sc.Task != string(ai.TaskAgentLoop) {
			continue
		}
		found++
		if rubric := strings.Join(strings.Fields(sc.Expect.Rubric), " "); !strings.Contains(rubric, agentLoopFirstStepOnly) {
			t.Errorf("agent_loop scenario %s: its rubric never says it grades the turn's %q; "+
				"open it with the sentence its siblings carry", sc.Name, agentLoopFirstStepOnly)
		}
	}
	if found == 0 {
		t.Fatalf("the corpus loaded no %s scenario, so no rubric was checked", ai.TaskAgentLoop)
	}
}
