// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

type meaningAnswer struct {
	State     string `json:"state"`
	Condition string `json:"condition"`
	Precision string `json:"deadline_precision"`
	Quote     string `json:"deadline_quote"`
}

type meaningExpectation struct {
	Meaning *meaningAnswer `json:"meaning"`
	Line    int            `json:"line"`
	Message int            `json:"message"`
	Kind    string         `json:"kind"`
	Due     string         `json:"due_date"`
}

func openMeaning() *meaningAnswer { return &meaningAnswer{State: "open", Precision: "none"} }

func eventMeaning(kind string) *meaningAnswer {
	if kind == "commitment_made" {
		return openMeaning()
	}
	return nil
}

func fixtureValue[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func semanticProofs(t *testing.T, scenarios map[string]aicert.Scenario) map[string]judgelessProof {
	t.Helper()
	out := map[string]judgelessProof{}
	for name, sc := range scenarios {
		if sc.Expect.Judged() || (sc.Task != "signal_extract" && sc.Task != "transcript_propose") {
			continue
		}
		expected := fixtureValue[[]meaningExpectation](t, sc.Expect.Answer)
		if len(expected) != 1 || expected[0].Meaning == nil {
			continue
		}
		want := expected[0]
		out[name] = judgelessProof{
			correct:   meaningReply(t, sc, want),
			wrong:     meaningReply(t, sc, meaningExpectation{Meaning: openMeaning(), Line: want.Line, Message: want.Message, Kind: want.Kind}),
			wantWrong: aitasks.OutcomeWrongAnswer,
		}
	}
	for _, language := range []string{"en", "de", "vi"} {

		name := "termination_notice_survives_without_a_request_" + language
		sc := proofScenario(t, scenarios, name)
		out[name] = judgelessProof{correct: event("contract_ended", 1, firstMeaningSource(t, sc)), wrong: literal(`{"events":[]}`), wantWrong: aitasks.OutcomeWrongAnswer}
		name = "intent_is_separate_from_sales_direction_" + language
		sc = proofScenario(t, scenarios, name)
		verdicts := fixtureValue[[]string](t, sc.Expect.Answer)
		wrong := append([]string(nil), verdicts...)
		wrong[0] = "asks_us"
		out[name] = judgelessProof{correct: owedVerdicts(verdicts...), wrong: owedVerdicts(wrong...), wantWrong: aitasks.OutcomeWrongAnswer}
		addPartialDeliveryProof(t, out, scenarios, language)
	}
	sc := proofScenario(t, scenarios, "tentative_promise_with_a_day")
	out[sc.Name] = judgelessProof{correct: emptyMeaningReply(sc.Task), wrong: meaningReply(t, sc, meaningExpectation{Meaning: openMeaning(), Line: 1}), wantWrong: aitasks.OutcomeWrongAnswer}
	return out
}

func proofScenario(t *testing.T, scenarios map[string]aicert.Scenario, name string) aicert.Scenario {
	t.Helper()
	sc, ok := scenarios[name]
	if !ok {
		t.Fatalf("proof scenario %q is absent", name)
	}
	return sc
}

func firstMeaningSource(t *testing.T, sc aicert.Scenario) string {
	t.Helper()
	if sc.Task == "transcript_propose" {
		return fixtureValue[[]string](t, sc.Fixture)[0]
	}
	messages := fixtureValue[[]struct {
		Body string `json:"body"`
	}](t, sc.Fixture)
	return messages[0].Body
}

func emptyMeaningReply(task string) func([]string) string {
	if task == "transcript_propose" {
		return literal(`{"proposals":[]}`)
	}
	return literal(`{"events":[]}`)
}

func meaningReply(t *testing.T, sc aicert.Scenario, want meaningExpectation) func([]string) string {
	t.Helper()
	source := firstMeaningSource(t, sc)
	if sc.Task == "transcript_propose" {
		return literal(mustJSON(map[string]any{"proposals": []map[string]any{{
			"summary": "Send the requested document.", "owner": "Mira", "promiser_ref": "", "source_lines": []int{want.Line}, "due_date": want.Due, "confidence": 0.95, "commitment": want.Meaning,
		}}}))
	}
	return func(ids []string) string {
		return mustJSON(map[string]any{"events": []map[string]any{{
			"kind": want.Kind, "message_id": ids[want.Message-1], "summary": "Send the requested document.", "quote": source, "promiser_ref": "", "due_date": want.Due, "confidence": 0.95, "commitment": want.Meaning,
		}}})
	}
}

func addPartialDeliveryProof(t *testing.T, out map[string]judgelessProof, scenarios map[string]aicert.Scenario, language string) {
	t.Helper()
	sc := proofScenario(t, scenarios, "partial_delivery_keeps_the_remaining_invoice_"+language)
	expected := fixtureValue[[]struct {
		Remaining string `json:"remaining_names"`
	}](t, sc.Expect.Answer)
	remaining, _, _ := strings.Cut(expected[0].Remaining, "|")
	// The correct answer quotes the request, so it takes the word before the
	// expected name too: a one-word remaining text is refused.
	fixture := fixtureValue[[]struct {
		Messages []struct{ Body string } `json:"messages"`
	}](t, sc.Fixture)
	body := fixture[0].Messages[0].Body
	at := strings.Index(body, remaining)
	if at < 0 {
		t.Fatalf("%s expects %q, which their request never says", sc.Name, remaining)
	}
	start := strings.LastIndexAny(strings.TrimSpace(body[:at]), " \t\r\n") + 1
	remaining = body[start : at+len(remaining)]
	out[sc.Name] = judgelessProof{correct: settlementReply(remaining, false), wrong: settlementReply(remaining, true), wantWrong: aitasks.OutcomeWrongAnswer}
}

func settlementReply(remaining string, eraseDebt bool) func([]string) string {
	return func(ids []string) string {
		verdict := "still_owed"
		words := remaining
		if eraseDebt {
			verdict, words = "settled", ""
		}
		return mustJSON(map[string]any{"results": []map[string]any{
			{"id": ids[0], "verdict": verdict, "remaining": words, "due_at": "", "confidence": 0.95},
			{"id": ids[1], "verdict": "settled", "remaining": "", "due_at": "", "confidence": 0.95},
		}})
	}
}
