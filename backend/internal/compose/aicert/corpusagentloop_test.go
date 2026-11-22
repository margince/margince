// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// An agent_loop case is graded on the run's writes. A judge shown the whole run
// (every read, observation and answer) grades whatever it notices, unless the
// rubric names what is graded. So a judged rubric names the write tool its
// scenario expects. A case graded by its mechanical check alone has no rubric.
func TestEveryAgentLoopRubricNamesTheWriteItGrades(t *testing.T) {
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
		if !sc.Expect.Judged() {
			continue
		}
		var write string
		if err := json.Unmarshal(sc.Expect.Answer, &write); err != nil {
			t.Errorf("agent_loop scenario %s: its answer is not a step name: %v", sc.Name, err)
			continue
		}
		if rubric := strings.Join(strings.Fields(sc.Expect.Rubric), " "); !strings.Contains(rubric, "Grade the") || !strings.Contains(rubric, write) {
			t.Errorf("agent_loop scenario %s: its rubric never says it grades what %s wrote; "+
				"open it with \"Grade the ... the run wrote with %s\"", sc.Name, write, write)
		}
	}
	if found == 0 {
		t.Fatalf("the corpus loaded no %s scenario, so no rubric was checked", ai.TaskAgentLoop)
	}
}
