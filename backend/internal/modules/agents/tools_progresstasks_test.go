// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Specs for what progress_deal answers about a closing deal's open tasks. Only
// a close asks, and it reports unless told to settle. A failed read or settle
// fails the call rather than answering as if the deal had no tasks.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type scriptedSettler struct {
	open    []crmcontracts.DealOpenTask
	settled []ids.UUID
	err     error
}

func (s scriptedSettler) OpenTasksOnDeal(context.Context, ids.UUID) ([]crmcontracts.DealOpenTask, error) {
	return s.open, s.err
}

func (s scriptedSettler) CompleteOpenTasksOnDeal(context.Context, ids.UUID) ([]ids.UUID, error) {
	return s.settled, s.err
}

type failingStages struct{ err error }

func (f failingStages) StageSemantic(context.Context, ids.UUID) (string, ids.UUID, error) {
	return "", ids.UUID{}, f.err
}

func TestAnOrdinaryStageMoveAsksNothingAboutTasks(t *testing.T) {
	tool := progressDeal{
		stages:     fixedStages{semantic: stageSemanticOpen},
		closeTasks: scriptedSettler{open: []crmcontracts.DealOpenTask{{Subject: "Send the contract"}}},
	}
	var result ProgressDealResult
	if err := tool.answerForOpenTasks(context.Background(), progressDealArgs{}, &result); err != nil {
		t.Fatal(err)
	}
	if result.OpenTasks != nil || result.SettledTaskIDs != nil {
		t.Errorf("a move to an open stage answered %+v, want nothing about tasks", result)
	}
}

func TestAClosingMoveReportsItsOpenTasksUnlessToldToSettle(t *testing.T) {
	open := []crmcontracts.DealOpenTask{{Subject: "Send the contract"}}
	settled := []ids.UUID{ids.NewV7()}
	tool := progressDeal{
		stages:     fixedStages{semantic: "won"},
		closeTasks: scriptedSettler{open: open, settled: settled},
	}

	var reported ProgressDealResult
	if err := tool.answerForOpenTasks(context.Background(), progressDealArgs{}, &reported); err != nil {
		t.Fatal(err)
	}
	if len(reported.OpenTasks) != 1 || reported.SettledTaskIDs != nil {
		t.Errorf("a close without settle answered %+v, want the open task reported and none settled", reported)
	}

	complete := settleComplete
	var settledResult ProgressDealResult
	if err := tool.answerForOpenTasks(context.Background(), progressDealArgs{Settle: &complete}, &settledResult); err != nil {
		t.Fatal(err)
	}
	if len(settledResult.SettledTaskIDs) != 1 || settledResult.OpenTasks != nil {
		t.Errorf("a close told to settle answered %+v, want the settled ids and no open list", settledResult)
	}
}

func TestAFailedTaskAnswerFailsTheClose(t *testing.T) {
	broken := errors.New("tasks unreadable")
	complete := settleComplete
	for _, tc := range []struct {
		name string
		tool progressDeal
		args progressDealArgs
	}{
		{"the stage cannot be read", progressDeal{stages: failingStages{err: broken}, closeTasks: scriptedSettler{}}, progressDealArgs{}},
		{"the open tasks cannot be read", progressDeal{stages: fixedStages{semantic: "lost"}, closeTasks: scriptedSettler{err: broken}}, progressDealArgs{}},
		{"the tasks cannot be settled", progressDeal{stages: fixedStages{semantic: "won"}, closeTasks: scriptedSettler{err: broken}}, progressDealArgs{Settle: &complete}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result ProgressDealResult
			if err := tc.tool.answerForOpenTasks(context.Background(), tc.args, &result); !errors.Is(err, broken) {
				t.Errorf("answer = %v, want the failure: answering empty says the deal had no tasks", err)
			}
		})
	}
}
