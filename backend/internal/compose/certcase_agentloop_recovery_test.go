// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func recoveryFixture(t *testing.T, kind string) agentLoopFixture {
	t.Helper()
	f := agentLoopBaseFixture(t)
	f.Band = agentLoopRecoveryBand
	if kind == "unsupported" {
		f.Tools = f.Tools[:1]
	} else {
		f.Tools = append(f.Tools, agentLoopToolAnswer{Tool: "read_record", Args: map[string]json.RawMessage{"id": json.RawMessage(`"` + agentLoopDealID.String() + `"`)}, Result: json.RawMessage(`{"record_type":"deal","id":"` + agentLoopDealID.String() + `","fields":{"name":"Heat recovery"}}`)})
	}
	f.Tools = append([]agentLoopToolAnswer{{Tool: "read_record", Refusal: &agentLoopToolRefusal{Kind: kind}}}, f.Tools...)
	return f
}

func recoveryRecordRead() string {
	return `{"tool":"read_record","args":{"record_type":"activity","id":"` + agentLoopActivityID.String() + `"}}`
}
func recoveryDealRead() string {
	return `{"tool":"read_record","args":{"record_type":"deal","id":"` + agentLoopDealID.String() + `"}}`
}

func measuredRecovery(t *testing.T, outcome aitasks.Outcome) aitasks.LoopRecoveryMeasure {
	t.Helper()
	if outcome.Recovery == nil {
		t.Fatal("the replay omitted its recovery measurement")
	}
	return *outcome.Recovery
}

func TestTheRecoveryBandRepairsRealRefusalsAlongTheWholeGoalPath(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"bad_arguments", "unknown_field", "field_shape"} {
		t.Run(kind, func(t *testing.T) {
			f := recoveryFixture(t, kind)
			outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, recoveryDealRead(), recoveryRecordRead(), annotation(agentLoopItemID, agentLoopActivityID), agentLoopFinalReply)
			if outcome.Result != aitasks.OutcomeAccepted {
				t.Fatal(outcome)
			}
			metrics := measuredRecovery(t, outcome)
			if !metrics.FirstStepCorrect || !metrics.Completed || metrics.Refusals != 1 || !metrics.Recovered || metrics.TerminalRepeated {
				t.Fatalf("metrics=%+v", metrics)
			}
			if len(trace.Requests) != agentLoopRecoverySteps-1 {
				t.Fatalf("the fixture-owned failed read was charged as a candidate call: %d requests", len(trace.Requests))
			}
			var run agentLoopRun
			if err := json.Unmarshal([]byte(trace.Output), &run); err != nil {
				t.Fatal(err)
			}
			if !run.SeededRefusal || len(run.Steps) != 5 {
				t.Fatalf("the trace omitted the seeded refusal or a required read: %+v", run)
			}
			if run.Steps[0].Admission != "refused" {
				t.Fatal("the candidate was never given a refusal to recover from")
			}
			var content strings.Builder
			for _, message := range trace.Requests[0].Messages {
				content.WriteString(message.Content)
			}
			if !strings.Contains(content.String(), "tool call refused:") || !strings.Contains(content.String(), "read_record") {
				t.Fatal("the first candidate turn never received the real refusal")
			}
			var output map[string]json.RawMessage
			if err := json.Unmarshal([]byte(trace.Output), &output); err != nil {
				t.Fatal(err)
			}
			if _, leaked := output["recovery"]; leaked {
				t.Fatal("the judge was given a code-computed recovery verdict")
			}
		})
	}
}

func TestTheRecoveryBandKeepsEveryRunnerTerminalRefusalTerminal(t *testing.T) {
	t.Parallel()
	f := recoveryFixture(t, "unsupported")
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopFinalStep, agentLoopFinalReply)
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatal(outcome)
	}
	metrics := measuredRecovery(t, outcome)
	if !metrics.FirstStepCorrect || !metrics.Completed || metrics.Refusals != 1 || metrics.Recovered || metrics.TerminalRepeated {
		t.Fatalf("metrics=%+v", metrics)
	}
	var content strings.Builder
	for _, message := range trace.Requests[0].Messages {
		content.WriteString(message.Content)
	}
	if !strings.Contains(content.String(), "do not call it again in this run") {
		t.Fatal("the first candidate turn missed the terminal directive")
	}
	repeated, _ := runAgentLoopScript(t, agentLoopAgent, f, agentLoopFinalStep, recoveryRecordRead(), agentLoopFinalReply)
	if repeated.Result == aitasks.OutcomeAccepted || !measuredRecovery(t, repeated).TerminalRepeated {
		t.Fatal("a final answer hid a repeated unsupported call")
	}
	f = recoveryFixture(t, "bad_arguments")
	outside := `{"tool":"send_email","args":{}}`
	repeated, _ = runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, outside, outside, agentLoopFinalReply)
	if repeated.Result == aitasks.OutcomeAccepted || !measuredRecovery(t, repeated).TerminalRepeated || measuredRecovery(t, repeated).Refusals != 1 {
		t.Fatalf("allowlist refusals were treated as repairable or as the seeded argument refusal: %+v", repeated)
	}
}

func TestAnUnrelatedMissingRecordIsNotARecoveredArgumentRefusal(t *testing.T) {
	t.Parallel()
	f := recoveryFixture(t, "bad_arguments")
	missing := `{"tool":"read_record","args":{"record_type":"activity","id":"0198f3a1-7c42-7e0b-9d51-2a6f4b8c1f99"}}`
	outcome, _ := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, missing, agentLoopFinalReply)
	metrics := measuredRecovery(t, outcome)
	if metrics.Refusals != 1 || metrics.Recovered || metrics.Completed {
		t.Fatalf("an ordinary miss counted as argument recovery: %+v", metrics)
	}
}

func TestAnUnrepairedRefusalCannotRunPastTheRecoveryBudget(t *testing.T) {
	t.Parallel()
	f := recoveryFixture(t, "field_shape")
	bad := `{"tool":"read_record","args":{"record_type":"activity","id":[]}}`
	outcome, trace := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, bad)
	if outcome.Result == aitasks.OutcomeAccepted || len(trace.Requests) != agentLoopRecoverySteps-1 || measuredRecovery(t, outcome).Completed {
		t.Fatalf("unbounded or accepted run: %+v, requests=%d", outcome, len(trace.Requests))
	}
}

func TestRecoveryCompletionNeedsTheGoalAndTheFirstCandidateReplyMustBeValid(t *testing.T) {
	t.Parallel()
	f := recoveryFixture(t, "bad_arguments")
	outcome, _ := runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, agentLoopReadBriefStep, agentLoopFinalReply)
	if outcome.Result == aitasks.OutcomeAccepted || measuredRecovery(t, outcome).Completed {
		t.Fatal("ending without the required write counted as completion")
	}
	outcome, _ = runAgentLoopScript(t, agentLoopAgent, f, agentLoopWriteTool, `{"tool":"read_brief","args":{},"extra":true}`, agentLoopReadBriefStep, agentLoopFinalReply)
	if measuredRecovery(t, outcome).FirstStepCorrect {
		t.Fatal("a malformed candidate reply became first-step success after a later valid call")
	}
}

type failingRecoveryCompleter struct{ script agentLoopScript }

func (c *failingRecoveryCompleter) Complete(ctx context.Context, request model.Request) (model.Response, error) {
	if len(c.script.requests) > 0 {
		return model.Response{}, errors.New("the model call failed")
	}
	return c.script.Complete(ctx, request)
}

func TestARealRecoveryCaseKeepsItsFirstCandidateStepAfterAModelFailure(t *testing.T) {
	t.Parallel()
	f := recoveryFixture(t, "bad_arguments")
	prepared, err := agentLoopCases{agent: agentLoopAgent}.Prepare(fixtureJSON(t, f), agentLoopExpectationJSON(t, agentLoopWriteTool))
	if err != nil {
		t.Fatal(err)
	}
	brain := &failingRecoveryCompleter{script: agentLoopScript{replies: []string{agentLoopReadBriefStep}}}
	trace, err := prepared.Run(t.Context(), brain)
	if err == nil || trace.Output == "" {
		t.Fatalf("partial trace was lost: error=%v output=%q", err, trace.Output)
	}
	metrics := prepared.(*agentLoopCase).RecoveryMeasure(trace)
	if metrics == nil || !metrics.FirstStepCorrect || metrics.Completed || metrics.Refusals != 1 {
		t.Fatalf("real failed-run measurement=%+v", metrics)
	}
}
