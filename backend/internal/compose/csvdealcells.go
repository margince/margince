// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A deal row's own cells, read the way the deals store takes them. The stage
// is found by name, the amount in minor units, the close date as a day.

import (
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/importrecords"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// checkCells reads a deal row's stage, amount, currency and close date into
// typed, or answers each of them the deals store would refuse. A row whose
// stage is refused has its close date read as a day and no more.
func (d dealStages) checkCells(fields map[string]string, typed map[string]any) (map[string]any, []*cellProblem) {
	if typed == nil {
		typed = map[string]any{}
	}
	var problems cellProblems
	choice, staged, problem := d.stageOf(fields)
	problems.add(problem)
	if staged {
		typed[dealTargetPipeline], typed[dealTargetStage] = choice.pipeline, choice.stage
	}
	problems.add(importrecords.CheckMoney(fields, typed, dealTargetAmount, dealTargetCurrency))
	problems.add(d.checkCloseDate(fields, choice, typed))
	if choice.semantic == crmcontracts.StageSemanticLost && strings.TrimSpace(fields[dealTargetLostReason]) == "" {
		problems.add(problemOf(dealTargetLostReason, fields[dealTargetLostReason],
			migration.Why(migration.ReasonLostReasonMissing)))
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return typed, nil
}

// stageOf is the stage a row names, and whether it names one. A row that maps
// a pipeline or a stage must name the stage.
func (d dealStages) stageOf(fields map[string]string) (stageChoice, bool, *cellProblem) {
	_, pipelineMapped := fields[dealTargetPipeline]
	stage, stageMapped := fields[dealTargetStage]
	if !pipelineMapped && !stageMapped {
		return stageChoice{}, false, nil
	}
	if strings.TrimSpace(stage) == "" {
		return stageChoice{}, false, problemOf(dealTargetStage, stage, migration.Why(migration.ReasonStageUnnamed))
	}
	choice, problem := d.resolve(fields[dealTargetPipeline], stage)
	return choice, problem == nil, problem
}

// closeDateHorizonYears is how far ahead a close date lies before the import
// asks whether it is a typing slip, such as 9999-12-31 for 2029-12-31.
const closeDateHorizonYears = 50

// farCloseDate is the warning a deal row lands with when its close date lies
// past the horizon. The row still lands: only its reader knows the date is wrong.
func (d dealStages) farCloseDate(fields map[string]string) *cellProblem {
	text := strings.TrimSpace(fields[dealTargetCloseDate])
	date, err := time.Parse(time.DateOnly, text)
	if err != nil || !date.After(d.today.AddDate(closeDateHorizonYears, 0, 0)) {
		return nil
	}
	return problemOf(dealTargetCloseDate, fields[dealTargetCloseDate],
		migration.Why(migration.ReasonCloseDateFar, "value", text, "years", strconv.Itoa(closeDateHorizonYears)))
}

// farCloseDateWarning is farCloseDate for one of this part's rows, as the
// warning the row lands with. Rows of other types have none.
func (w *csvWriters) farCloseDateWarning(row migration.Row) []migration.SkippedRow {
	if w.object != migration.ObjectDeal {
		return nil
	}
	problem := w.dealStages.farCloseDate(textFields(row.Fields))
	if problem == nil {
		return nil
	}
	return []migration.SkippedRow{{
		ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(problem.Target), Value: problem.Value,
		Reason: problem.Message, Code: problem.Why.Code, Params: problem.Why.Params,
	}}
}

// closedSemantic is whether a stage ends a deal; the zero semantic of a row
// naming no stage is neither.
func closedSemantic(s crmcontracts.StageSemantic) bool {
	return s == crmcontracts.StageSemanticWon || s == crmcontracts.StageSemanticLost
}

// checkCloseDate reads the close date as a day. An open deal's may not be in
// the past; a won or lost deal's may not be in the future.
// A row naming no stage leaves that to the deals store.
func (d dealStages) checkCloseDate(fields map[string]string, choice stageChoice, typed map[string]any) *cellProblem {
	text := strings.TrimSpace(fields[dealTargetCloseDate])
	if text == "" {
		return nil
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return problemOf(dealTargetCloseDate, fields[dealTargetCloseDate],
			migration.Why(migration.ReasonNotADate, "value", text))
	}
	if choice.semantic == crmcontracts.StageSemanticOpen && deals.CloseDateIsPast(date, d.today) {
		return problemOf(dealTargetCloseDate, fields[dealTargetCloseDate],
			migration.Why(migration.ReasonCloseDatePast, "value", text))
	}
	if closedSemantic(choice.semantic) && deals.CloseDateIsFuture(date, d.today) {
		return problemOf(dealTargetCloseDate, fields[dealTargetCloseDate],
			migration.Why(migration.ReasonCloseDateFuture, "value", text))
	}
	typed[dealTargetCloseDate] = openapi_types.Date{Time: date}
	return nil
}
