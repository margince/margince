// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A deal row's own cells, read the way the deals store takes them. The stage
// is found by name, the amount in minor units, the close date as a day.

import (
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// checkCells reads a deal row's stage, amount, currency and close date into
// typed. The first problem wins.
func (d dealStages) checkCells(fields map[string]string, typed map[string]any) (map[string]any, *cellProblem) {
	if typed == nil {
		typed = map[string]any{}
	}
	choice, staged, problem := d.stageOf(fields)
	if problem != nil {
		return nil, problem
	}
	if staged {
		typed[dealTargetPipeline], typed[dealTargetStage] = choice.pipeline, choice.stage
	}
	if problem := checkDealMoney(fields, typed); problem != nil {
		return nil, problem
	}
	if problem := d.checkCloseDate(fields, choice, typed); problem != nil {
		return nil, problem
	}
	if choice.semantic == crmcontracts.StageSemanticLost && strings.TrimSpace(fields[dealTargetLostReason]) == "" {
		return nil, problemOf(dealTargetLostReason, fields[dealTargetLostReason],
			migration.Why(migration.ReasonLostReasonMissing))
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

// checkDealMoney reads the amount in the minor units of its currency, which
// must be named beside it.
func checkDealMoney(fields map[string]string, typed map[string]any) *cellProblem {
	amount := strings.TrimSpace(fields[dealTargetAmount])
	currency := strings.ToUpper(strings.TrimSpace(fields[dealTargetCurrency]))
	if currency != "" {
		if !values.AssignableCurrency(currency) {
			return problemOf(dealTargetCurrency, fields[dealTargetCurrency],
				migration.Why(migration.ReasonCurrencyNotACode, "value", currency))
		}
		typed[dealTargetCurrency] = currency
	}
	if amount == "" {
		return nil
	}
	if currency == "" {
		return problemOf(dealTargetAmount, fields[dealTargetAmount],
			migration.Why(migration.ReasonCurrencyUnset, "value", amount))
	}
	minor, ok := values.MinorUnits(amount, currency)
	if !ok {
		return problemOf(dealTargetAmount, fields[dealTargetAmount],
			migration.Why(migration.ReasonNotAnAmount, "value", amount, "currency", currency))
	}
	if minor < 0 {
		return problemOf(dealTargetAmount, fields[dealTargetAmount],
			migration.Why(migration.ReasonAmountNegative, "value", amount))
	}
	typed[dealTargetAmount] = minor
	return nil
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
