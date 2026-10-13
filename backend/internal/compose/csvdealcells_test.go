// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageFixture is a stage of a pipeline the tests build.
type stageFixture struct {
	id       ids.UUID
	name     string
	semantic crmcontracts.StageSemantic
}

func pipelineOf(name string, isDefault bool, stages ...stageFixture) crmcontracts.Pipeline {
	out := make([]crmcontracts.Stage, 0, len(stages))
	for _, s := range stages {
		out = append(out, crmcontracts.Stage{Id: openapi_types.UUID(s.id), Name: s.name, Semantic: s.semantic})
	}
	return crmcontracts.Pipeline{Id: openapi_types.UUID(ids.NewV7()), Name: name, IsDefault: isDefault, Stages: &out}
}

var (
	salesQualified = stageFixture{ids.NewV7(), "Qualified", crmcontracts.StageSemanticOpen}
	salesWon       = stageFixture{ids.NewV7(), "Won", crmcontracts.StageSemanticWon}
	salesLost      = stageFixture{ids.NewV7(), "Lost", crmcontracts.StageSemanticLost}
	renewalClosing = stageFixture{ids.NewV7(), "Closing", crmcontracts.StageSemanticOpen}
	importToday    = time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
)

func testDealStages() dealStages {
	return dealStagesFrom([]crmcontracts.Pipeline{
		pipelineOf("Sales", true, salesQualified, salesWon, salesLost),
		pipelineOf("Renewals", false, renewalClosing),
	}, importToday)
}

func TestAStageIsFoundByNameInItsOwnPipelineOnly(t *testing.T) {
	stages := testDealStages()
	typed, problem := stages.checkCells(map[string]string{dealTargetPipeline: " sales ", dealTargetStage: "QUALIFIED"}, nil)
	if problem != nil || typed[dealTargetStage] != ids.From[ids.StageKind](salesQualified.id) {
		t.Fatalf("typed = %v, problem = %+v; want Sales' Qualified, whatever its case and spaces", typed, problem)
	}
	_, problem = stages.checkCells(map[string]string{dealTargetPipeline: "Sales", dealTargetStage: "Closing"}, nil)
	if problem == nil || problem.Target != dealTargetStage || !strings.Contains(problem.Message, `"Sales"`) {
		t.Fatalf("problem = %+v, want the stage refused naming the pipeline it is not in", problem)
	}
	_, problem = stages.checkCells(map[string]string{dealTargetPipeline: "Upsell", dealTargetStage: "Won"}, nil)
	if problem == nil || problem.Target != dealTargetPipeline || !strings.Contains(problem.Message, "Renewals") {
		t.Fatalf("problem = %+v, want an unknown pipeline refused with the pipelines there are", problem)
	}
}

func TestADealWithNoPipelineGoesToTheDefaultOne(t *testing.T) {
	typed, problem := testDealStages().checkCells(map[string]string{dealTargetStage: "won", dealTargetCloseDate: "2026-07-10"}, nil)
	if problem != nil || typed[dealTargetStage] != ids.From[ids.StageKind](salesWon.id) {
		t.Fatalf("typed = %v, problem = %+v; want the default pipeline's Won", typed, problem)
	}
}

func TestAStageCannotBeCheckedWithoutReadingPipelines(t *testing.T) {
	_, problem := dealStages{today: importToday}.checkCells(map[string]string{dealTargetStage: "Won"}, nil)
	if problem == nil || problem.Target != dealTargetStage || !strings.Contains(problem.Message, "pipelines") {
		t.Fatalf("problem = %+v, want the stage refused for want of the pipeline grant", problem)
	}
}

func TestAnAmountIsReadInItsCurrencysMinorUnits(t *testing.T) {
	stages := testDealStages()
	for _, tc := range []struct{ amount, currency string }{{"12000.50", "EUR"}, {"1500000", "vnd"}} {
		typed, problem := stages.checkCells(map[string]string{dealTargetAmount: tc.amount, dealTargetCurrency: tc.currency}, nil)
		want := map[string]int64{"EUR": 1200050, "vnd": 1500000}[tc.currency]
		if problem != nil || typed[dealTargetAmount] != want || typed[dealTargetCurrency] != strings.ToUpper(tc.currency) {
			t.Errorf("%s %s: typed = %v, problem = %+v; want %d in the code's own spelling", tc.amount, tc.currency, typed, problem, want)
		}
	}
	for _, tc := range []struct{ amount, currency, target, says string }{
		{"12.5", "JPY", dealTargetAmount, "12000"},
		{"12.000,50", "EUR", dealTargetAmount, "12000.50"},
		{"12000", "", dealTargetAmount, "currency"},
		{"12000", "Euro", dealTargetCurrency, "three letters"},
	} {
		_, problem := stages.checkCells(map[string]string{dealTargetAmount: tc.amount, dealTargetCurrency: tc.currency}, nil)
		if problem == nil || problem.Target != tc.target || !strings.Contains(problem.Message, tc.says) {
			t.Errorf("%q %q: problem = %+v, want one on %s saying %q", tc.amount, tc.currency, problem, tc.target, tc.says)
		}
	}
}

func TestAnOpenDealWithAPastCloseDateIsAProblem(t *testing.T) {
	_, problem := testDealStages().checkCells(map[string]string{dealTargetStage: "Qualified", dealTargetCloseDate: "2026-07-03"}, nil)
	if problem == nil || problem.Target != dealTargetCloseDate || !strings.Contains(problem.Message, "open deal") {
		t.Fatalf("problem = %+v, want the past date refused on an open deal", problem)
	}
	if _, problem = testDealStages().checkCells(map[string]string{dealTargetStage: "Qualified", dealTargetCloseDate: "03/07/2026"}, nil); problem == nil ||
		!strings.Contains(problem.Message, "YYYY-MM-DD") {
		t.Fatalf("problem = %+v, want a date in another shape refused with the shape", problem)
	}
}

func TestAClosedDealMayCloseInThePast(t *testing.T) {
	cells := map[string]string{dealTargetStage: "Lost", dealTargetCloseDate: "2025-06-30", dealTargetLostReason: "Too expensive"}
	typed, problem := testDealStages().checkCells(cells, nil)
	want := openapi_types.Date{Time: time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)}
	if problem != nil || typed[dealTargetCloseDate] != want {
		t.Fatalf("typed = %v, problem = %+v; want the past date kept on a lost deal", typed, problem)
	}
	delete(cells, dealTargetLostReason)
	if _, problem = testDealStages().checkCells(cells, nil); problem == nil || problem.Target != dealTargetLostReason {
		t.Fatalf("problem = %+v, want a lost deal without its reason refused on lost_reason", problem)
	}
}
