// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// Every request the corpus exercises alternates user and assistant turns and
// opens with the user.
//
// The chat templates of Mistral's and Gemma's instruction models raise on any
// other order, and a server that renders the model's own template (vLLM)
// refuses the whole call with a 400 — so a site that sends two user turns in a
// row works on Ollama and on every cloud vendor, and is unservable on those
// models the moment an operator binds them through vLLM.
//
// The subject is the request each scenario's own certification case builds, so
// a site joins this gate by joining the corpus. A scenario whose request cannot
// be built fails here rather than dropping out of the count.
func TestEveryCorpusRequestAlternatesTurns(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("LoadCorpus(corpus): %v", err)
	}
	if len(scenarios) == 0 {
		t.Fatal("the corpus holds no scenario, so this gate is holding nothing")
	}
	for _, sc := range scenarios {
		req, err := firstBuiltRequest(context.Background(), sc, census)
		if err != nil {
			t.Errorf("%s: %v", sc.Name, err)
			continue
		}
		roles := make([]string, len(req.Messages))
		for i, m := range req.Messages {
			roles[i] = m.Role
		}
		if len(roles) > 0 && roles[0] != "user" {
			t.Errorf("%s (%s/%s) opens with a %q turn: %s", sc.Name, sc.Task, sc.Site, roles[0], strings.Join(roles, ","))
		}
		for i := 1; i < len(roles); i++ {
			if roles[i] == roles[i-1] {
				t.Errorf("%s (%s/%s) sends two %q turns in a row: %s — join them (compose's alternatingTurns) "+
					"or a Mistral or Gemma model served by vLLM refuses the call",
					sc.Name, sc.Task, sc.Site, roles[i], strings.Join(roles, ","))
				break
			}
		}
	}
}
