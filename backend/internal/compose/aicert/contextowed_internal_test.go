// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Internal because the subject is the request each case builds, which only
// firstBuiltRequest reaches without a paid run.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Every scenario whose request production prepends company context to is
// certified with that context. The case serves the demo company through the
// production provider. So the stamp and the record describe the prompt an
// installation with its own company record sends.
func TestEveryCaseProductionSendsCompanyContextToIsServedIt(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	owedScenarios := 0
	for _, sc := range corpus {
		req, err := firstBuiltRequest(context.Background(), sc, census)
		if err != nil {
			t.Fatalf("building %s/%s: %v", sc.Task, sc.Name, err)
		}
		if !requestOwesContext(ai.Task(sc.Task), req) {
			continue
		}
		owedScenarios++
		if req.ContextFingerprint == "" {
			t.Errorf("%s/%s/%s: production prepends company context to this request and the case sends it bare",
				sc.Task, sc.Site, sc.Name)
		}
	}
	// A census that found nothing owed would pass by reading nothing.
	if owedScenarios == 0 {
		t.Fatal("no scenario's request is owed company context, so this test checked nothing")
	}
}

// A conditional policy is owed only to a request that asks; an unconditional
// one to every request; a task declaring no scopes to none.
func TestARequestOwesContextOnlyWhereProductionSendsIt(t *testing.T) {
	asked := model.Request{IncludeCompanyContext: true}
	prepared := model.Request{ContextScopes: []string{"identity"}}
	for _, tc := range []struct {
		name string
		task ai.Task
		req  model.Request
		want bool
	}{
		{"a conditional task nobody asked for", ai.TaskSummarize, model.Request{}, false},
		{"a conditional task its caller asked for", ai.TaskAccountScan, asked, true},
		{"a conditional task the provider already prepared", ai.TaskNlSearch, prepared, true},
		{"an unconditional task", ai.TaskDraftReply, model.Request{}, true},
		{"a task declaring no scopes", ai.TaskRateExtract, asked, false},
	} {
		if got := requestOwesContext(tc.task, tc.req); got != tc.want {
			t.Errorf("%s: requestOwesContext = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// summarize declares a conditional identity scope that no site of it asks for.
// So the report says nothing about it, and the tasks whose callers ask are owed.
func TestContextOwedTasksReadsTheCallSites(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	owed, err := ContextOwedTasks(context.Background(), corpus, census)
	if err != nil {
		t.Fatalf("ContextOwedTasks: %v", err)
	}
	for task, want := range map[string]bool{
		"summarize": false, "account_scan": true, "nl_search": true, "draft_reply": true, "offer_draft": true, "growth_fit": true,
	} {
		if owed[task] != want {
			t.Errorf("%s owed company context = %v, want %v", task, owed[task], want)
		}
	}
}
