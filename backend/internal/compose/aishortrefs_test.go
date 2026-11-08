// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stillShowsRecordIDs names the sites whose request still carries a record
// UUID, each with where it carries one. The list only shrinks: an entry whose
// site no longer shows one fails, so a converted site leaves it in the same
// change.
//
// gatekit:fixture the sites not yet moved to promptref — expected data the gate checks still holds, not a waived cost
var stillShowsRecordIDs = map[string]string{
	"account_scan/company_scan":              "contact ids in the account JSON and the schema",
	"agent_loop/morning_brief":               "seed context record ids, passed back to tools",
	"agent_loop/overnight_at_risk_sweep":     "seed context record ids, passed back to tools",
	"brief_ranking/rank":                     "candidate ids",
	"capture_confidentiality_verdict/thread": "the thread id on its one span; integration fakes key on it",
	"capture_counterparty_verdict/verdict":   "the sender id on its one span; integration fakes key on it",
	"corpus_ask/corpus_ask":                  "passage ids",
	"deal_health/deal_status":                "the deal JSON ids; integration fakes reply with them",
	"deal_verdict/deal_verdict":              "activity, stage and criterion ids; integration fakes reply with them",
	"enrich/signature":                       "the source activity id on its span",
	"growth_fit/growth_fit":                  "profile field ids",
	"offer_draft/draft":                      "product ids in the catalog",
	"request_settlement/request_settle":      "request ids; integration replies are scripted by id",
	"stage_evidence_extract/criteria":        "span and criterion ids",
	"summarize/company_dossier":              "profile field ids",
	"summarize/contact_brief":                "record ids in the contact JSON",
	"summarize/deal_room_brief":              "record ids in the deal room JSON",
	"summarize/meeting_brief":                "activity ids in the meeting JSON",
	"summarize/meeting_plan":                 "deal and contact ids in the plan JSON",
	"tag_suggest/interest":                   "the source record id on each span",
	"weekly_learnings/learn":                 "deal ids in the week JSON",
}

// A model is never shown a record UUID: it copies one wrong, cites it in the
// wrong field or writes it into prose, and a small model does all three. A
// site names its records by short refs from promptref and reads a reply's refs
// back to ids in its parser. This walks every request every registered site
// issues for every corpus scenario; docs/how-to/add-an-ai-task.md has the steps.
func TestNoSiteShowsTheModelARecordUUID(t *testing.T) {
	t.Parallel()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	shown := map[string][]string{}
	walked := map[string]int{}
	for _, sc := range scenarios {
		key := sc.Task + "/" + sc.Site
		for i, req := range scenarioRequests(t, census, sc) {
			walked[key]++
			if where := recordIDsIn(req); where != "" {
				shown[key] = append(shown[key], fmt.Sprintf("%s call %d: %s", sc.Name, i+1, where))
			}
		}
	}
	for _, site := range census.All() {
		key := string(site.Task) + "/" + site.Variant
		if walked[key] == 0 {
			t.Errorf("site %s issued no request over its corpus, so this gate read nothing of it", key)
		}
	}
	for key, found := range shown {
		if _, pending := stillShowsRecordIDs[key]; pending {
			continue
		}
		sort.Strings(found)
		t.Errorf("site %s shows the model a record UUID (%s). Name each record by a short ref from "+
			"promptref.Refs (backend/internal/shared/kernel/promptref) and read the reply's refs back in "+
			"its parser; docs/how-to/add-an-ai-task.md shows how", key, found[0])
	}
	for key := range stillShowsRecordIDs {
		if len(shown[key]) == 0 {
			t.Errorf("site %s no longer shows a record UUID: remove it from stillShowsRecordIDs", key)
		}
	}
}

// scenarioRequests returns the requests a site's case issues for one scenario,
// answered by the stand-in reply the canary gate uses. A case that refuses
// that reply still issued the requests before the refusal.
func scenarioRequests(t *testing.T, census *aitasks.Registry, sc aicert.Scenario) []model.Request {
	t.Helper()
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		t.Errorf("%s/%s: no case is bound to this site", sc.Task, sc.Site)
		return nil
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		t.Errorf("%s/%s (%s): the site refused its own committed fixture: %v", sc.Task, sc.Site, sc.Name, err)
		return nil
	}
	recorder := &recordingCompleter{reply: canaryReply}
	if _, runErr := prepared.Run(context.Background(), recorder); runErr != nil {
		t.Logf("%s/%s (%s): the case refused the stand-in reply after %d request(s): %v",
			sc.Task, sc.Site, sc.Name, len(recorder.requests), runErr)
	}
	return recorder.requests
}

// recordIDsIn names each model-facing part of a request that carries a record
// UUID, or "" when none does.
func recordIDsIn(req model.Request) string {
	var where []string
	if loc := mintedID.FindStringIndex(req.System); loc != nil {
		where = append(where, "system: …"+around(req.System, loc))
	}
	for _, m := range req.Messages {
		if loc := mintedID.FindStringIndex(m.Content); loc != nil {
			where = append(where, "messages: …"+around(m.Content, loc))
			break
		}
	}
	if mintedID.Match(req.ResponseSchema) {
		where = append(where, "response schema")
	}
	for _, tool := range req.Tools {
		if mintedID.Match(tool.InputSchema) || mintedID.MatchString(tool.Description) {
			where = append(where, "tool "+tool.Name)
		}
	}
	return strings.Join(where, "; ")
}

// around is the text just before a match and the match itself, so a failure
// shows which field carried the id.
func around(text string, loc []int) string {
	return text[max(0, loc[0]-40):loc[1]]
}
