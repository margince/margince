// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A preset's certification, rung by rung: for every task, the record behind the
// model that answers and the one behind the model a failed call falls to. A
// missing fallback record is a gap a buyer meets on the first failed call, so it
// is reported exactly like a missing first-rung record.

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// The fallback states that name no record: the next rung binds the model that
// just failed, or the ladder binds no next rung at all.
const (
	RungSameModel = "same model"
	RungNone      = "none"
)

// PresetRungState is one task's first rung and fallback under one preset, with
// the readiness state of the record that grades each.
type PresetRungState struct {
	Task          ai.Task
	FirstTier     ai.Tier
	FirstModel    string
	FirstState    string
	FallbackTier  ai.Tier
	FallbackModel string
	FallbackState string
}

// PresetRungs is routing's first rung and fallback for every task the corpus
// certifies, each with the state `make e2e-ai-report` gives its record.
func PresetRungs(ctx context.Context, routing ai.RoutingConfig, corpus []Scenario, census *aitasks.Registry, records []Record) ([]PresetRungState, error) {
	byTask := map[ai.Task][]Scenario{}
	for _, sc := range corpus {
		byTask[ai.Task(sc.Task)] = append(byTask[ai.Task(sc.Task)], sc)
	}
	tasks := make([]ai.Task, 0, len(byTask))
	for task := range byTask {
		tasks = append(tasks, task)
	}
	slices.Sort(tasks)
	var states []PresetRungState
	for _, task := range tasks {
		if len(boundLadder(routing, task)) == 0 {
			continue
		}
		rows, err := readinessOf(ctx, census, task, byTask[task], records)
		if err != nil {
			return nil, err
		}
		states = append(states, presetRungState(routing, task, byTask[task], rows))
	}
	return states, nil
}

// presetRungState reads task's two leading rungs under routing against rows.
func presetRungState(routing ai.RoutingConfig, task ai.Task, scenarios []Scenario, rows []ReadinessRow) PresetRungState {
	rungs := boundLadder(routing, task)
	first := rungs[0]
	state := PresetRungState{
		Task: task, FirstTier: first.Tier, FirstModel: first.Binding.Model,
		FirstState:    rungState(rows, scenarios, first.Binding, routing.Profile, task),
		FallbackState: RungNone,
	}
	if len(rungs) < 2 {
		return state
	}
	next := rungs[1]
	state.FallbackTier, state.FallbackModel = next.Tier, next.Binding.Model
	if sameModel(first.Binding, next.Binding) {
		state.FallbackState = RungSameModel
		return state
	}
	state.FallbackState = rungState(rows, scenarios, next.Binding, routing.Profile, task)
	return state
}

// statusRank orders the states worst first, so a record current on one site
// and stale on another reads stale: the rung is only as current as its worst site.
var statusRank = map[string]int{StatusAbsent: 0, StatusStale: 1, StatusPartial: 2, StatusCurrent: 3}

// rungState is the worst state across the sites task's scenarios run on of the
// record that grades binding; a site no such record measured is absent.
func rungState(rows []ReadinessRow, scenarios []Scenario, binding ai.ProviderConfig, profile ai.Profile, task ai.Task) string {
	worst := StatusCurrent
	for _, site := range scenarioSites(scenarios) {
		status := StatusAbsent
		for _, row := range rows {
			if row.Site.Variant == site && row.Certified && recordMeasures(row.Record, binding, profile, task) {
				status = row.Status()
				break
			}
		}
		if statusRank[status] < statusRank[worst] {
			worst = status
		}
	}
	return worst
}
