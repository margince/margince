// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

type lifecycleExpectation struct {
	Kind       string `json:"kind"`
	Message    int    `json:"message"`
	Scope      string `json:"lifecycle_scope"`
	Workstream string `json:"workstream"`
}

func lifecycleProofs(t *testing.T, scenarios map[string]aicert.Scenario) map[string]judgelessProof {
	t.Helper()
	out := map[string]judgelessProof{}
	for name, scenario := range scenarios {
		if scenario.Expect.Judged() || scenario.Task != "signal_extract" {
			continue
		}
		expected := fixtureValue[[]lifecycleExpectation](t, scenario.Expect.Answer)
		if len(expected) != 1 || expected[0].Scope == "" {
			continue
		}
		correct := expected[0]
		wrong := correct
		wrong.Scope, wrong.Workstream = "company", ""
		if correct.Scope == "company" {
			wrong.Scope = "workstream"
		}
		quote := firstMeaningSource(t, scenario)
		out[name] = judgelessProof{
			correct: lifecycleReply(correct, quote), wrong: lifecycleReply(wrong, quote),
			wantWrong: aitasks.OutcomeWrongAnswer,
		}
	}
	return out
}

func lifecycleReply(want lifecycleExpectation, quote string) func([]string) string {
	return func(ids []string) string {
		return mustJSON(map[string]any{"events": []map[string]any{{
			"kind": want.Kind, "message_id": ids[want.Message-1], "summary": "The source reports an ending.",
			"quote": quote, "confidence": 1.0, "lifecycle_scope": want.Scope, "workstream": want.Workstream,
		}}})
	}
}
