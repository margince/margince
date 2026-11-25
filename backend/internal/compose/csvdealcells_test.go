// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
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
	typed, problem := firstOf(stages.checkCells(map[string]string{dealTargetPipeline: " sales ", dealTargetStage: "QUALIFIED"}, nil))
	if problem != nil || typed[dealTargetStage] != ids.From[ids.StageKind](salesQualified.id) {
		t.Fatalf("typed = %v, problem = %+v; want Sales' Qualified, whatever its case and spaces", typed, problem)
	}
	_, problem = firstOf(stages.checkCells(map[string]string{dealTargetPipeline: "Sales", dealTargetStage: "Closing"}, nil))
	if problem == nil || problem.Target != dealTargetStage || !strings.Contains(problem.Message, "“Sales”") {
		t.Fatalf("problem = %+v, want the stage refused naming the pipeline it is not in", problem)
	}
	_, problem = firstOf(stages.checkCells(map[string]string{dealTargetPipeline: "Upsell", dealTargetStage: "Won"}, nil))
	if problem == nil || problem.Target != dealTargetPipeline || !strings.Contains(problem.Message, "Renewals") {
		t.Fatalf("problem = %+v, want an unknown pipeline refused with the pipelines there are", problem)
	}
}

func TestADealWithNoPipelineGoesToTheDefaultOne(t *testing.T) {
	typed, problem := firstOf(testDealStages().checkCells(map[string]string{dealTargetStage: "won", dealTargetCloseDate: "2026-07-01"}, nil))
	if problem != nil || typed[dealTargetStage] != ids.From[ids.StageKind](salesWon.id) {
		t.Fatalf("typed = %v, problem = %+v; want the default pipeline's Won", typed, problem)
	}
}

func TestAStageCannotBeCheckedWithoutReadingPipelines(t *testing.T) {
	_, problem := firstOf(dealStages{today: importToday}.checkCells(map[string]string{dealTargetStage: "Won"}, nil))
	if problem == nil || problem.Target != dealTargetStage || !strings.Contains(problem.Message, "pipelines") {
		t.Fatalf("problem = %+v, want the stage refused for want of the pipeline grant", problem)
	}
}

func TestAnAmountIsReadInItsCurrencysMinorUnits(t *testing.T) {
	stages := testDealStages()
	for _, tc := range []struct{ amount, currency string }{{"12000.50", "EUR"}, {"1500000", "vnd"}} {
		typed, problem := firstOf(stages.checkCells(map[string]string{dealTargetAmount: tc.amount, dealTargetCurrency: tc.currency}, nil))
		want := map[string]int64{"EUR": 1200050, "vnd": 1500000}[tc.currency]
		if problem != nil || typed[dealTargetAmount] != want || typed[dealTargetCurrency] != strings.ToUpper(tc.currency) {
			t.Errorf("%s %s: typed = %v, problem = %+v; want %d in the code's own spelling", tc.amount, tc.currency, typed, problem, want)
		}
	}
	for _, tc := range []struct{ amount, currency, target, says string }{
		{"1000.50", "JPY", dealTargetAmount, "has more decimal places than JPY takes. Write it like 12000."},
		{"10.12345", "EUR", dealTargetAmount, "has more decimal places than EUR takes. Write it like 12000.50."},
		{"5.5", "KWD", dealTargetAmount, ""},
		{"-500", "EUR", dealTargetAmount, "“-500” is below zero"},
		{"1e5", "KWD", dealTargetAmount, "is not an amount in KWD. Write it like 12000.500."},
		{"12.000,50", "EUR", dealTargetAmount, "is not an amount in EUR. Write it like 12000.50."},
		{"12000", "", dealTargetAmount, "Fill the currency column"},
		{"12000", "Euro", dealTargetCurrency, "three letters"},
	} {
		_, problem := firstOf(stages.checkCells(map[string]string{dealTargetAmount: tc.amount, dealTargetCurrency: tc.currency}, nil))
		if tc.says == "" {
			if problem != nil {
				t.Errorf("%q %q: problem = %+v, want it read", tc.amount, tc.currency, problem)
			}
			continue
		}
		if problem == nil || problem.Target != tc.target || !strings.Contains(problem.Message, tc.says) {
			t.Errorf("%q %q: problem = %+v, want one on %s saying %q", tc.amount, tc.currency, problem, tc.target, tc.says)
		}
	}
}

func TestAnOpenDealWithAPastCloseDateIsAProblem(t *testing.T) {
	_, problem := firstOf(testDealStages().checkCells(map[string]string{dealTargetStage: "Qualified", dealTargetCloseDate: "2026-07-03"}, nil))
	if problem == nil || problem.Target != dealTargetCloseDate || !strings.Contains(problem.Message, "open deal") {
		t.Fatalf("problem = %+v, want the past date refused on an open deal", problem)
	}
	if _, problem = firstOf(testDealStages().checkCells(map[string]string{dealTargetStage: "Qualified", dealTargetCloseDate: "03/07/2026"}, nil)); problem == nil ||
		!strings.Contains(problem.Message, "YYYY-MM-DD") {
		t.Fatalf("problem = %+v, want a date in another shape refused with the shape", problem)
	}
}

func TestAClosedDealMayCloseInThePast(t *testing.T) {
	cells := map[string]string{dealTargetStage: "Lost", dealTargetCloseDate: "2025-06-30", dealTargetLostReason: "Too expensive"}
	typed, problem := firstOf(testDealStages().checkCells(cells, nil))
	want := openapi_types.Date{Time: time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)}
	if problem != nil || typed[dealTargetCloseDate] != want {
		t.Fatalf("typed = %v, problem = %+v; want the past date kept on a lost deal", typed, problem)
	}
	delete(cells, dealTargetLostReason)
	if _, problem = firstOf(testDealStages().checkCells(cells, nil)); problem == nil || problem.Target != dealTargetLostReason {
		t.Fatalf("problem = %+v, want a lost deal without its reason refused on lost_reason", problem)
	}
}

func TestAClosedDealWithACloseDateAfterTodayIsRefusedAndTodayIsNot(t *testing.T) {
	stages := testDealStages()
	for _, stage := range []string{"Won", "Lost"} {
		fields := map[string]string{dealTargetPipeline: "Sales", dealTargetStage: stage, dealTargetCloseDate: "2026-07-05", dealTargetLostReason: "price"}
		_, problem := firstOf(stages.checkCells(fields, nil))
		if problem == nil || problem.Why.Code != migration.ReasonCloseDateFuture {
			t.Errorf("%s with a close date tomorrow: problem = %+v, want close_date_future", stage, problem)
		}
		fields[dealTargetCloseDate] = "2026-07-04"
		if _, problem := firstOf(stages.checkCells(fields, nil)); problem != nil {
			t.Errorf("%s closed today: problem = %+v, want it accepted", stage, problem)
		}
	}
	open := map[string]string{dealTargetPipeline: "Sales", dealTargetStage: "Qualified", dealTargetCloseDate: "2026-07-05"}
	if _, problem := firstOf(stages.checkCells(open, nil)); problem != nil {
		t.Errorf("an open deal's future close date: problem = %+v, want it accepted", problem)
	}
}

// A deal row with its amount and its close date both wrong names both cells.
func TestADealRowNamesEveryCellAtFault(t *testing.T) {
	fields := map[string]string{dealTargetStage: "Qualified", dealTargetAmount: "1.234,56", dealTargetCurrency: "EUR", dealTargetCloseDate: "31.03.2027"}
	typed, problems := testDealStages().checkCells(fields, nil)
	if typed != nil || len(problems) != 2 || problems[0].Target != dealTargetAmount || problems[1].Target != dealTargetCloseDate {
		t.Fatalf("typed = %v, problems = %+v; want the amount and the close date refused", typed, problems)
	}
}
