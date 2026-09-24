// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a pinned argument owes a corpus author: it is compared as a subset
// claim, numbers are compared by what they mean, and every assertion no call
// could ever satisfy is refused at Prepare instead of at a paid run.
//
// Each refusal here is paired with the case it exists to catch, because a guard
// that passes against the bug it describes is a guard nobody notices is gone.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The tool these tests pin arguments on: one morning_brief is offered, so a
// rename reaches this file the same commit it reaches the agent's window.
const (
	agentLoopPinnedTool = "catch_me_up_on"
	agentLoopPinnedArg  = "record_id"
	agentLoopDealID     = `"0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07"`
)

// runAgentLoopExpectation drives one case whose expectation is written out as
// JSON, which is how a corpus scenario carrying arguments reaches Prepare.
func runAgentLoopExpectation(
	t *testing.T, fixture agentLoopFixture, expected json.RawMessage, reply string,
) aitasks.Outcome {
	t.Helper()
	prepared, err := agentLoopCases{agent: agentLoopAgent}.Prepare(agentLoopFixtureJSON(t, fixture), expected)
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), &replyBrainStub{response: model.Response{Text: reply}})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

// agentLoopArgExpectation writes the object spelling of an expected step.
func agentLoopArgExpectation(t *testing.T, step, args string) json.RawMessage {
	t.Helper()
	return json.RawMessage(`{"step":"` + step + `","args":` + args + `}`)
}

// The scenarios that pin arguments pin them as a SUBSET: every argument named
// has to be carried as named, and an argument the scenario never named is the
// call being richer than its author imagined rather than wrong.
func TestAgentLoopCaseGradesThePinnedArgumentsAndNoOthers(t *testing.T) {
	pinned := `{"record_id":` + agentLoopDealID + `}`
	for _, tc := range []struct {
		name       string
		reply      string
		wantResult string
		wantDetail string
	}{
		{
			name:       "the call carries the pinned argument",
			reply:      `{"tool":"catch_me_up_on","args":{"record_type":"deal","record_id":` + agentLoopDealID + `}}`,
			wantResult: aitasks.OutcomeAccepted,
			wantDetail: `the turn took the step "catch_me_up_on"`,
		},
		{
			name:       "the call carries more than the scenario pinned",
			reply:      `{"tool":"catch_me_up_on","args":{"record_type":"deal","record_id":` + agentLoopDealID + `,"max_items":5}}`,
			wantResult: aitasks.OutcomeAccepted,
			wantDetail: `the turn took the step "catch_me_up_on"`,
		},
		{
			name:       "the call omits the pinned argument",
			reply:      `{"tool":"catch_me_up_on","args":{"record_type":"deal"}}`,
			wantResult: aitasks.OutcomeWrongAnswer,
			wantDetail: agentLoopPinnedArg + " was not passed",
		},
		{
			name:       "the call passes the pinned argument as something else",
			reply:      `{"tool":"catch_me_up_on","args":{"record_type":"deal","record_id":"the big deal"}}`,
			wantResult: aitasks.OutcomeWrongAnswer,
			wantDetail: `record_id was passed as "the big deal", not ` + agentLoopDealID,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := runAgentLoopExpectation(t, agentLoopBaseFixture(),
				agentLoopArgExpectation(t, agentLoopPinnedTool, pinned), tc.reply)
			if outcome.Result != tc.wantResult {
				t.Fatalf("Result = %q (%s), want %q", outcome.Result, outcome.Detail, tc.wantResult)
			}
			if !strings.Contains(outcome.Detail, tc.wantDetail) {
				t.Errorf("Detail = %q, want it to name %q", outcome.Detail, tc.wantDetail)
			}
		})
	}
}

// Numbers are the one JSON value with two spellings of the same meaning and two
// meanings that share a spelling once rounded. A pinned max_items is a real
// argument on this window, so both halves are reachable from a scenario:
// float64 would grade the two integers below as equal, and comparing literals
// would grade 5 and 5.0 as different.
func TestAgentLoopCaseComparesPinnedNumbersExactly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       string
		reply      string
		wantResult string
	}{
		{
			name:       "the same number written two ways agrees",
			args:       `{"max_items":5}`,
			reply:      `{"tool":"catch_me_up_on","args":{"record_type":"deal","record_id":` + agentLoopDealID + `,"max_items":5.0}}`,
			wantResult: aitasks.OutcomeAccepted,
		},
		{
			name:       "two integers past float64's reach stay apart",
			args:       `{"max_items":9007199254740993}`,
			reply:      `{"tool":"catch_me_up_on","args":{"max_items":9007199254740992}}`,
			wantResult: aitasks.OutcomeWrongAnswer,
		},
		{
			name:       "a number nested in a pinned object is compared too",
			args:       `{"max_items":{"page":9007199254740993}}`,
			reply:      `{"tool":"catch_me_up_on","args":{"max_items":{"page":9007199254740992}}}`,
			wantResult: aitasks.OutcomeWrongAnswer,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := runAgentLoopExpectation(t, agentLoopBaseFixture(),
				agentLoopArgExpectation(t, agentLoopPinnedTool, tc.args), tc.reply)
			if outcome.Result != tc.wantResult {
				t.Errorf("Result = %q (%s), want %q", outcome.Result, outcome.Detail, tc.wantResult)
			}
		})
	}
}

// A wrong step is a wrong step. Reporting what a call the scenario never wanted
// got wrong about its arguments would bury the one thing that went wrong under
// the details of a call that should not have been made.
func TestAgentLoopCaseReportsTheWrongStepWithoutItsArguments(t *testing.T) {
	outcome := runAgentLoopExpectation(t, agentLoopBaseFixture(),
		agentLoopArgExpectation(t, agentLoopPinnedTool, `{"record_id":`+agentLoopDealID+`}`),
		`{"tool":"read_record","args":{"record_type":"deal","id":"018f3a1b-0000-7000-8000-000000000010"}}`)

	if outcome.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("Result = %q (%s), want %q", outcome.Result, outcome.Detail, aitasks.OutcomeWrongAnswer)
	}
	if !strings.Contains(outcome.Detail, `took the step "read_record" where the scenario expects "catch_me_up_on"`) {
		t.Errorf("Detail = %q, want it to name the step disagreement", outcome.Detail)
	}
	if strings.Contains(outcome.Detail, agentLoopPinnedArg+" was not passed") {
		t.Errorf("Detail = %q, want no argument disagreement under a wrong step", outcome.Detail)
	}
}

// An argument assertion the turn could never satisfy fails for the scenario's
// reason rather than the model's, and it would do so on every paid run until
// somebody read the record closely enough to notice.
func TestAgentLoopCaseRefusesAnArgumentNoCallCouldCarry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expected json.RawMessage
		want     string
	}{
		{
			name:     "arguments pinned on the step that ends a run",
			expected: agentLoopArgExpectation(t, agentLoopFinalStep, `{"record_id":"x"}`),
			want:     "the step that ends a run carries none",
		},
		{
			name:     "an argument the tool's own schema does not declare",
			expected: agentLoopArgExpectation(t, agentLoopPinnedTool, `{"deal_id":"x"}`),
			want:     `pins the argument "deal_id" on catch_me_up_on`,
		},
		{
			name:     "an expectation object carrying a key this site does not know",
			expected: json.RawMessage(`{"step":"catch_me_up_on","arguments":{"record_id":"x"}}`),
			want:     "not a step name",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := agentLoopCases{agent: agentLoopAgent}.Prepare(agentLoopFixtureJSON(t, agentLoopBaseFixture()), tc.expected)
			if err == nil {
				t.Fatal("Prepare accepted an argument assertion this site cannot measure")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal reads %q, want it to name %q", err, tc.want)
			}
		})
	}
}
