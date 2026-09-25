// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestTriageCriteriaNameExactlyTheTriageKinds(t *testing.T) {
	for _, kind := range siteTriageKinds {
		if strings.TrimSpace(triageDecisionCriteria[kind]) == "" {
			t.Errorf("kind %q has no criterion, so the decision model can never be told when it is right", kind)
		}
	}
	for label := range triageDecisionCriteria {
		if !slices.Contains(siteTriageKinds, label) {
			t.Errorf("criterion %q names no triage kind; an answer of it would be refused as off-enum every time", label)
		}
	}
	if got, want := slices.Sorted(maps.Keys(triageDecisionCriteria)), slices.Sorted(slices.Values(siteTriageKinds)); !slices.Equal(got, want) {
		t.Errorf("criteria labels %v, want exactly the triage kinds %v", got, want)
	}
}

// triageDecisionPageOf reads the page the decision state carries.
func triageDecisionPageOf(t *testing.T, req decision.Request) triageDecisionPage {
	t.Helper()
	var state triageDecisionState
	if err := json.Unmarshal(req.State, &state); err != nil {
		t.Fatalf("the decision state is not the triage state: %v", err)
	}
	return state.Page
}

// jsonLeafPaths lists every scalar's dotted path in a JSON object, so a state
// that grew a field is seen whatever the field's name.
func jsonLeafPaths(t *testing.T, prefix string, raw json.RawMessage) []string {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return []string{prefix}
	}
	var paths []string
	for key, value := range object {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		paths = append(paths, jsonLeafPaths(t, path, value)...)
	}
	return paths
}

func TestTriageDecisionStateCarriesTheLLMInputs(t *testing.T) {
	const urlCanary = "https://qzvurlcanary.example"
	const headCanary = "qzvHEADcanary"
	const textCanary = "qzvTEXTcanary"
	page := crawlPage{URL: urlCanary, HeadText: []string{headCanary}, Text: textCanary}

	req := triageRequest(page, string(textlang.English))
	dreq := triageDecision(page)
	state := string(dreq.State)
	for _, canary := range []string{urlCanary, headCanary, textCanary} {
		if !strings.Contains(req.Messages[0].Content, canary) {
			t.Fatalf("the LLM request lost %q, so this test no longer measures its inputs", canary)
		}
		if !strings.Contains(state, canary) {
			t.Errorf("the LLM request carries %q and the decision state does not", canary)
		}
	}
	if got := slices.Sorted(slices.Values(jsonLeafPaths(t, "", dreq.State))); !slices.Equal(got, []string{"page.text", "page.url"}) {
		t.Errorf("decision state leaves %v, want exactly page.text and page.url — the LLM is sent nothing else", got)
	}
}

func TestTriageDecisionStateExcerptsThePageAsTheLLMRequestDoes(t *testing.T) {
	long := strings.Repeat("Ω", 5_000)
	page := crawlPage{URL: "https://acme.example", Text: long}

	sent := triageDecisionPageOf(t, triageDecision(page)).Text
	if got := len([]rune(sent)); got != triageExcerptRunes {
		t.Errorf("the decision state carries %d runes, want the %d the excerpt keeps", got, triageExcerptRunes)
	}
	body := triageRequest(page, string(textlang.English)).Messages[0].Content
	if !strings.Contains(body, sent) || strings.Count(body, "Ω") != len([]rune(sent)) {
		t.Error("the decision state and the LLM request carry different excerpts of the same page")
	}
}

func TestTriageDecisionGateUsesTheSiteFloor(t *testing.T) {
	for _, kind := range siteTriageKinds {
		if got := triageDecisionGate(decision.Answer{Choice: kind, Confidence: 0.79}); got != ai.DecisionBelowFloor {
			t.Errorf("%s at 0.79 = %v, want below the floor", kind, got)
		}
		if got := triageDecisionGate(decision.Answer{Choice: kind, Confidence: triageAbortConfidence}); got != ai.DecisionAccepted {
			t.Errorf("%s at the floor = %v, want accepted", kind, got)
		}
	}
	if got := triageDecisionGate(decision.Answer{Choice: "bogus", Confidence: 0.99}); got != ai.DecisionOffEnum {
		t.Errorf("an unknown label = %v, want off-enum however confident", got)
	}
}

// scriptedDecidingLane is a lane that can decide: it gates its scripted answer
// through the site's own gate as the router does, and answers with the LLM
// reply when the gate does not accept it.
type scriptedDecidingLane struct {
	answer   decision.Answer
	llmReply string

	askedSite string
	asked     decision.Request
	llmCalled bool
}

func (l *scriptedDecidingLane) CompleteDecided(_ context.Context, site string, dreq decision.Request,
	_ model.Request, _ ai.Validator, gate ai.DecisionGate,
) (ai.DecideOutcome, error) {
	l.askedSite, l.asked = site, dreq
	if gate(l.answer) == ai.DecisionAccepted {
		return ai.DecideOutcome{Decided: true, Answer: l.answer}, nil
	}
	l.llmCalled = true
	return ai.DecideOutcome{Response: model.Response{Text: l.llmReply}}, nil
}

func (l *scriptedDecidingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.llmCalled = true
	return model.Response{Text: l.llmReply}, nil
}

const triageLLMReply = `{"kind":"company","confidence":0.9,"reason":"it sells robots"}`

func TestClassifySeedTakesAConfidentDecision(t *testing.T) {
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: siteKindParked, Confidence: 0.93},
		llmReply: triageLLMReply,
	}
	seed := crawlPage{URL: "https://acme.example", Text: "This domain is for sale."}

	verdict, err := (&siteDeepReadWorker{triageBrain: lane}).classifySeed(context.Background(), seed)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if lane.llmCalled {
		t.Error("the LLM was asked although the decision stood")
	}
	if verdict.Kind != siteKindParked || float64(verdict.Confidence) != 0.93 || verdict.Reason != "" {
		t.Errorf("verdict %+v, want the decision's parked at 0.93 with no prose", verdict)
	}
	if lane.askedSite != triageDecisionSite {
		t.Errorf("asked at site %q, want %q", lane.askedSite, triageDecisionSite)
	}
	if got := triageDecisionPageOf(t, lane.asked); got.URL != seed.URL || got.Text != seed.Text {
		t.Errorf("the decision was asked about %+v, want the seed page", got)
	}
}

func TestClassifySeedFallsBackBelowTheFloor(t *testing.T) {
	lane := &scriptedDecidingLane{
		answer:   decision.Answer{Choice: siteKindParked, Confidence: 0.6},
		llmReply: triageLLMReply,
	}
	seed := crawlPage{URL: "https://acme.example", Text: "We build robots."}

	verdict, err := (&siteDeepReadWorker{triageBrain: lane}).classifySeed(context.Background(), seed)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if !lane.llmCalled {
		t.Error("an unsure decision was kept instead of falling back to the LLM")
	}
	if verdict.Kind != siteKindCompany || verdict.Reason != "it sells robots" {
		t.Errorf("verdict %+v, want the LLM's own company answer", verdict)
	}
}

// completerOnlyLane can only complete, as the offline fake and the debug
// recorder can; the worker must still classify through it.
type completerOnlyLane struct{ reply string }

func (l completerOnlyLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}

func TestClassifySeedAsksTheLLMOnALaneThatCannotDecide(t *testing.T) {
	seed := crawlPage{URL: "https://acme.example", Text: "We build robots."}
	worker := &siteDeepReadWorker{triageBrain: completerOnlyLane{reply: triageLLMReply}}

	verdict, err := worker.classifySeed(context.Background(), seed)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if verdict.Kind != siteKindCompany || verdict.Reason != "it sells robots" {
		t.Errorf("verdict %+v, want the LLM's answer", verdict)
	}
}
