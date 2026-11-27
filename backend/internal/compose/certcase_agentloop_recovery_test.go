// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func recoveryFixture(t *testing.T, kind string) (agentLoopFixture, string) {
	t.Helper()
	f := agentLoopBaseFixture(t)
	f.Band = agentLoopRecoveryBand
	refusal := &agentLoopToolRefusal{Kind: kind, Field: "id"}
	args := map[string]json.RawMessage{"id": json.RawMessage(`"not-a-record-id"`)}
	switch kind {
	case "unknown_field":
		refusal.Field = "obsolete_field"
		args = map[string]json.RawMessage{"id": json.RawMessage(`"` + agentLoopActivityID.String() + `"`), "obsolete_field": json.RawMessage(`true`)}
	case "field_shape":
		refusal.Want = "a UUID string"
		refusal.Got = "an array"
		args["id"] = json.RawMessage(`[]`)
	case "unsupported":
		refusal.Field = ""
		args = nil
	}
	f.Tools = append([]agentLoopToolAnswer{{Tool: "read_record", Args: args, Refusal: refusal}}, f.Tools...)
	bad := fixtureJSON(t, map[string]any{"tool": "read_record", "args": args})
	if kind == "unsupported" {
		bad = json.RawMessage(`{"tool":"read_record","args":{"record_type":"activity","id":"` + agentLoopActivityID.String() + `"}}`)
	}
	return f, string(bad)
}

func recoveryRecordRead() string {
	return `{"tool":"read_record","args":{"record_type":"activity","id":"` + agentLoopActivityID.String() + `"}}`
}

func recordedRecovery(t *testing.T, trace aitasks.Trace) agentLoopRecoveryMeasure {
	t.Helper()
	var run agentLoopRun
	if err := json.Unmarshal([]byte(trace.Output), &run); err != nil {
		t.Fatal(err)
	}
	if run.Recovery == nil {
		t.Fatal("recovery metrics were not recorded")
	}
	return *run.Recovery
}

func TestTheRecoveryBandRepairsTypedRefusalsWithinFiveModelSteps(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"bad_arguments", "unknown_field", "field_shape"} {
		t.Run(kind, func(t *testing.T) {
			f, bad := recoveryFixture(t, kind)
			outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, bad, recoveryRecordRead(), annotation(agentLoopItemID, agentLoopActivityID), agentLoopFinalReply)
			if outcome.Result != aitasks.OutcomeAccepted {
				t.Fatal(outcome)
			}
			metrics := recordedRecovery(t, trace)
			if !metrics.FirstStepCorrect || !metrics.Completed || metrics.Refusals != 1 || !metrics.Recovered || metrics.TerminalRepeated {
				t.Fatalf("metrics=%+v", metrics)
			}
			if len(trace.Requests) != agentLoopRecoverySteps {
				t.Fatalf("used %d turns", len(trace.Requests))
			}
			window, err := json.Marshal(trace.Requests[2])
			if err != nil {
				t.Fatal(err)
			}
			needle := "the argument value was not accepted"
			if kind == "unknown_field" {
				needle = "use the declared arguments"
			}
			if kind == "field_shape" {
				needle = "must be a UUID string"
			}
			if !strings.Contains(string(window), needle) {
				t.Fatalf("the next model turn never received its refusal: %s", window)
			}
			if !strings.Contains(outcome.Detail, "first_step_correct") || !strings.Contains(outcome.Detail, "completed") {
				t.Fatalf("the grade merged first-step accuracy with completion: %s", outcome.Detail)
			}
		})
	}
}

func TestTheRecoveryBandKeepsAnUnsupportedToolTerminal(t *testing.T) {
	t.Parallel()
	f, bad := recoveryFixture(t, "unsupported")
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopFinalStep, agentLoopReadBriefStep, bad, agentLoopFinalReply)
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatal(outcome)
	}
	metrics := recordedRecovery(t, trace)
	if !metrics.Completed || metrics.Refusals != 1 || metrics.Recovered || metrics.TerminalRepeated {
		t.Fatalf("metrics=%+v", metrics)
	}
	window, err := json.Marshal(trace.Requests[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(window), "do not call it again in this run") {
		t.Fatalf("terminal directive missing: %s", window)
	}
	repeated, repeatedTrace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopFinalStep, agentLoopReadBriefStep, bad, bad, agentLoopFinalReply)
	if repeated.Result == aitasks.OutcomeAccepted || !recordedRecovery(t, repeatedTrace).TerminalRepeated {
		t.Fatalf("a final answer hid repeated terminal refusals: %+v", repeated)
	}
}

func TestTheRecoveryBandSeparatesAnAvoidedErrorFromARecoveredOne(t *testing.T) {
	t.Parallel()
	f, _ := recoveryFixture(t, "bad_arguments")
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, recoveryRecordRead(), annotation(agentLoopItemID, agentLoopActivityID), agentLoopFinalReply)
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatal(outcome)
	}
	metrics := recordedRecovery(t, trace)
	if !metrics.FirstStepCorrect || !metrics.Completed || metrics.Refusals != 0 || metrics.Recovered {
		t.Fatalf("avoiding a bad call was reported as recovery: %+v", metrics)
	}
}

func TestAnUnrepairedRefusalCannotRunPastTheRecoveryBudget(t *testing.T) {
	t.Parallel()
	f, bad := recoveryFixture(t, "field_shape")
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, bad)
	if outcome.Result == aitasks.OutcomeAccepted || len(trace.Requests) != agentLoopRecoverySteps {
		t.Fatalf("unbounded or accepted run: %+v, turns=%d", outcome, len(trace.Requests))
	}
	if recordedRecovery(t, trace).Completed {
		t.Fatal("budget exhaustion was recorded as completion")
	}
}

func TestRecoveryCompletionNeedsTheGoalAndTheFirstReplyMustReallyBeValid(t *testing.T) {
	t.Parallel()
	f, _ := recoveryFixture(t, "bad_arguments")
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, agentLoopFinalReply)
	if outcome.Result == aitasks.OutcomeAccepted || recordedRecovery(t, trace).Completed {
		t.Fatal("ending without the required write counted as completion")
	}
	outcome, trace = runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, `{"tool":"read_brief","args":{},"extra":true}`, agentLoopReadBriefStep, recoveryRecordRead(), annotation(agentLoopItemID, agentLoopActivityID), agentLoopFinalReply)
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatal(outcome)
	}
	if recordedRecovery(t, trace).FirstStepCorrect {
		t.Fatal("a malformed first reply became first-step success after a later valid call")
	}
}
