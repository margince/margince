// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every AI task declares how it treats a restriction of processing (GDPR
// Art. 18), read from the generated task table, never from a list typed here.
// honour makes the Router refuse a restricted contact before any provider is
// asked; none must say why the task reads no contact's data.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

func TestEveryAITaskDeclaresHowItTreatsARestriction(t *testing.T) {
	t.Parallel()
	tasks := ai.AllTasks()
	if len(tasks) == 0 {
		t.Fatal("the generated task table lists no task; the census read nothing")
	}
	honouring := 0
	for _, task := range tasks {
		posture := ai.SubjectRestrictionOf(task)
		switch posture.Posture {
		case "honour":
			honouring++
		case "none":
			if strings.TrimSpace(posture.Reason) == "" {
				t.Errorf("task %s declares subject_restriction none with no reason", task)
			}
		default:
			t.Errorf("task %s declares subject_restriction %q; want honour or none", task, posture.Posture)
		}
	}
	if honouring == 0 {
		t.Error("no AI task honours a restriction of processing; the mail tasks must")
	}
}
