// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A deal row's own cells, read the way the deals store takes them. The stage
// is found by name, the amount in minor units, the close date as a day.

import (
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
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
		return nil, &cellProblem{
			Target: dealTargetLostReason, Value: fields[dealTargetLostReason],
			Message: "a lost deal needs the reason it was lost",
		}
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
		return stageChoice{}, false, &cellProblem{Target: dealTargetStage, Value: stage, Message: "name the deal's stage"}
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
		if !values.ValidCurrency(currency) {
			return &cellProblem{
				Target: dealTargetCurrency, Value: fields[dealTargetCurrency],
				Message: fmt.Sprintf("%q is not a currency code; write its three letters, like EUR", currency),
			}
		}
		typed[dealTargetCurrency] = currency
	}
	if amount == "" {
		return nil
	}
	if currency == "" {
		return &cellProblem{
			Target: dealTargetAmount, Value: fields[dealTargetAmount],
			Message: fmt.Sprintf("%q needs a currency: fill the currency column, like EUR", amount),
		}
	}
	minor, ok := values.MinorUnits(amount, currency)
	if !ok {
		scale := values.MinorUnitScale(currency)
		return &cellProblem{
			Target: dealTargetAmount, Value: fields[dealTargetAmount],
			Message: fmt.Sprintf("%q is not an amount in %s; write it like %s", amount, currency,
				values.MajorUnits(12000*scale+scale/2, currency)),
		}
	}
	typed[dealTargetAmount] = minor
	return nil
}

// checkCloseDate reads the close date as a day. An open deal's may not be in
// the past; a row naming no stage leaves that to the deals store's write.
func (d dealStages) checkCloseDate(fields map[string]string, choice stageChoice, typed map[string]any) *cellProblem {
	text := strings.TrimSpace(fields[dealTargetCloseDate])
	if text == "" {
		return nil
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return &cellProblem{
			Target: dealTargetCloseDate, Value: fields[dealTargetCloseDate],
			Message: fmt.Sprintf("%q is not a date; write it as YYYY-MM-DD", text),
		}
	}
	if choice.semantic == crmcontracts.StageSemanticOpen && deals.CloseDateIsPast(date, d.today) {
		return &cellProblem{
			Target: dealTargetCloseDate, Value: fields[dealTargetCloseDate],
			Message: fmt.Sprintf("%q is in the past, and an open deal's close date cannot be", text),
		}
	}
	typed[dealTargetCloseDate] = openapi_types.Date{Time: date}
	return nil
}
