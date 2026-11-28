// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func TestRecoveryRatesDoNotMixOrdinaryRunsOrHideRepairedFirstSteps(t *testing.T) {
	t.Parallel()
	tally := summarizeRecovery([]RunResult{
		{},
		{Recovery: &aitasks.LoopRecoveryMeasure{FirstStepCorrect: true, Completed: true}},
		{Recovery: &aitasks.LoopRecoveryMeasure{Completed: true, Refusals: 1, Recovered: true}},
		{Recovery: &aitasks.LoopRecoveryMeasure{Refusals: 2, TerminalRepeated: true}},
	})
	if tally == nil || tally.Runs != 3 || tally.FirstStepCorrect != 1 || tally.Completed != 2 || tally.RunsWithRefusal != 2 || tally.Refusals != 3 || tally.RecoveredRuns != 1 || tally.TerminalRepeatedRuns != 1 {
		t.Fatalf("tally=%+v", tally)
	}
	if summarizeRecovery([]RunResult{{}}) != nil {
		t.Fatal("an ordinary run invented a recovery measurement")
	}
}

// withheldRecoveryCase models a run whose first tool call succeeded before its
// later model reply was withheld. The failed run still belongs in the rate.
type withheldRecoveryCase struct{}

func (withheldRecoveryCase) Run(context.Context, aitasks.Completer) (aitasks.Trace, error) {
	return aitasks.Trace{}, nil
}

func (withheldRecoveryCase) Evaluate(aitasks.Trace) aitasks.Outcome {
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func (withheldRecoveryCase) RecoveryMeasure(aitasks.Trace) *aitasks.LoopRecoveryMeasure {
	return &aitasks.LoopRecoveryMeasure{FirstStepCorrect: true, Completed: true, Recovered: true}
}

func TestAWithheldRecoveryRunRemainsInTheCompletionDenominator(t *testing.T) {
	t.Parallel()
	scored, err := scoreAnswer(t.Context(), withheldRecoveryCase{}, aitasks.ScopeSingleTurn, aitasks.Trace{}, Scenario{}, ai.TaskAgentLoop, runCalls{Withheld: "output_ceiling"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	measure := scored.outcome.Recovery
	if measure == nil || !measure.FirstStepCorrect || measure.Completed || measure.Recovered {
		t.Fatalf("withheld measurement=%+v", measure)
	}
	tally := summarizeRecovery([]RunResult{scored.outcome.RunResult})
	if tally == nil || tally.Runs != 1 || tally.Completed != 0 {
		t.Fatalf("withheld run disappeared: %+v", tally)
	}
}
