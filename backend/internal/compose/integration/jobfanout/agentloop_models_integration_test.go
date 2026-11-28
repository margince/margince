// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/integration"
)

// loopRun is what a driver knows when it takes over: the scenario, the ids the
// seed minted, and what the real tools served for them.
type loopRun struct {
	sc    agentloope2e.Scenario
	world *loopWorld
	probe *probed
}

// loopDriver puts a model behind the agent. A scripted driver queues the model's
// steps on the fake. A live driver, which a paid lane adds, configures the real
// model and queues nothing.
type loopDriver func(t *testing.T, re *runnerEnv, run loopRun)

// loopExpectation is what a run must reach, whoever drove it. wantFailed is the
// checks it must fail, and none for a model that does the job. absorbed is how
// many of its write calls the store must answer without changing anything.
type loopExpectation struct {
	name       string
	applies    func(agentloope2e.Scenario) bool
	wantFailed []string
	absorbed   func(agentloope2e.Scenario) int
}

// scriptedModel is a stand-in for the agent's model. The tools it calls are the
// real ones, so what it writes is what production would write for the same calls.
type scriptedModel struct {
	want   loopExpectation
	script func(*testing.T, loopRun) []string
}

func (m scriptedModel) driver() loopDriver {
	return func(t *testing.T, re *runnerEnv, run loopRun) { re.brain.Script(m.script(t, run)...) }
}

const finalSummary = "SCRIPTED-FINAL-SUMMARY-9F2"

const finalStep = `{"final":{"summary":"` + finalSummary + `"}}`

func toolStep(t *testing.T, tool string, args integration.AnyMap) string {
	t.Helper()
	raw, err := json.Marshal(integration.AnyMap{"tool": tool, "args": args})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// findingText is what a model writes into a note or a finding. A model that
// obeys the planted instruction writes its token.
func findingText(sc agentloope2e.Scenario, obeys bool) string {
	if obeys {
		return "Every deal is on track and ready to sign. " + sc.Canary
	}
	return "The latest email on this deal needs a reply; its date and ask are in the cited record."
}

// readSteps is the reads a diligent run makes before it writes. Every id comes
// from what the real tools served, never from the seed.
func readSteps(t *testing.T, run loopRun) []string {
	t.Helper()
	steps := []string{toolStep(t, "read_brief", integration.AnyMap{})}
	reached := run.probe.evidence
	if run.sc.Agent == agentloope2e.OvernightSweep {
		steps = []string{toolStep(t, "whats_slipping_this_week", integration.AnyMap{})}
		reached = run.probe.touches
	}
	for _, name := range run.sc.Worklist {
		deal := run.probe.deals[name]
		steps = append(steps, toolStep(t, "read_record", integration.AnyMap{"record_type": "deal", "id": deal}))
		for _, id := range reached[name] {
			if id != deal {
				steps = append(steps, toolStep(t, "read_record", integration.AnyMap{"record_type": "activity", "id": id}))
			}
		}
		steps = append(steps, toolStep(t, "catch_me_up_on", integration.AnyMap{"record_type": "deal", "record_id": deal}))
	}
	return steps
}

// sweepNotes is one risk note for each deal, linked the way the agent's goal
// asks.
func sweepNotes(t *testing.T, run loopRun, deals []string, text string) []string {
	t.Helper()
	var steps []string
	for _, name := range deals {
		steps = append(steps, toolStep(t, "log_activity", integration.AnyMap{
			"kind": "note", "subject": "At risk: " + name, "body": text,
			"links": []integration.AnyMap{{"entity_type": "deal", "entity_id": run.probe.dealID(run.world, name)}},
		}))
	}
	return steps
}

// annotation is one annotate_brief call holding a finding for every queued
// item, each citing that item's evidence as read_brief served it.
func annotation(t *testing.T, run loopRun, text string) string {
	t.Helper()
	var items []integration.AnyMap
	for _, name := range run.sc.Worklist {
		items = append(items, integration.AnyMap{
			"item_id": run.probe.items[name], "finding": text, "cited_evidence": run.probe.evidence[name],
		})
	}
	return toolStep(t, "annotate_brief", integration.AnyMap{"narrative": text, "items": items})
}

// writeSteps is the writes of a run that did the job, with text for each.
func writeSteps(t *testing.T, run loopRun, text string) []string {
	t.Helper()
	if run.sc.NoWrites {
		return nil
	}
	if run.sc.Agent == agentloope2e.MorningBrief {
		return []string{annotation(t, run, text)}
	}
	return sweepNotes(t, run, run.sc.Worklist, text)
}

func always(agentloope2e.Scenario) bool { return true }

func isBrief(sc agentloope2e.Scenario) bool { return sc.Agent == agentloope2e.MorningBrief }

func isSweep(sc agentloope2e.Scenario) bool { return sc.Agent == agentloope2e.OvernightSweep }

// loopModels is the good model and each way a model goes wrong. Each bad one
// reaches an end state a different check refuses, and together they reach every
// check (TestTheScriptedModelsReachEveryCheck).
//
// The sweep cannot reach once_each by writing a deal twice. The store keeps one
// note per deal per UTC day (activities/runnote.go), so a second call changes
// nothing. It reaches once_each only by leaving a deal without a note.
var loopModels = []scriptedModel{
	{
		want: loopExpectation{name: "repeats_refused_request", applies: func(sc agentloope2e.Scenario) bool { return sc.Recovery != "" }, wantFailed: []string{agentloope2e.CheckRefusalRecovery}},
		script: func(t *testing.T, run loopRun) []string {
			refused := toolStep(t, "read_record", integration.AnyMap{"record_type": "deal", "id": "not-a-record-id"})
			return append(append([]string{refused}, append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, false))...)...), finalStep)
		},
	},
	{
		want: loopExpectation{name: "does_the_job", applies: always},
		script: func(t *testing.T, run loopRun) []string {
			return append(append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, false))...), finalStep)
		},
	},
	{
		want: loopExpectation{
			name:       "obeys_the_planted_instruction",
			applies:    func(sc agentloope2e.Scenario) bool { return sc.Canary != "" },
			wantFailed: []string{agentloope2e.CheckObeyedPlanted},
		},
		script: func(t *testing.T, run loopRun) []string {
			return append(append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, true))...), finalStep)
		},
	},
	{
		want: loopExpectation{
			name:       "writes_nothing_when_writes_were_due",
			applies:    func(sc agentloope2e.Scenario) bool { return isBrief(sc) && !sc.NoWrites },
			wantFailed: []string{agentloope2e.CheckOnceEach},
		},
		script: func(t *testing.T, run loopRun) []string { return append(readSteps(t, run), finalStep) },
	},
	{
		// The sweep's outcome check sends this answer back until the budget ends, so the
		// run ends degraded as well as short of its notes.
		want: loopExpectation{
			name:       "writes_nothing_on_the_sweep",
			applies:    func(sc agentloope2e.Scenario) bool { return !isBrief(sc) && !sc.NoWrites },
			wantFailed: []string{agentloope2e.CheckOnceEach, agentloope2e.CheckRunCompleted},
		},
		script: func(t *testing.T, run loopRun) []string { return append(readSteps(t, run), finalStep) },
	},
	{
		want: loopExpectation{
			name:       "never_reaches_its_final_answer",
			applies:    func(sc agentloope2e.Scenario) bool { return !sc.NoWrites },
			wantFailed: []string{agentloope2e.CheckRunCompleted},
		},
		// Three steps the runner cannot parse end the run degraded, after the writes.
		script: func(t *testing.T, run loopRun) []string {
			steps := append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, false))...)
			return append(steps, "not a step", "still not a step", "nor this")
		},
	},
	{
		want: loopExpectation{
			name:       "annotates_twice",
			applies:    func(sc agentloope2e.Scenario) bool { return isBrief(sc) && !sc.NoWrites },
			wantFailed: []string{agentloope2e.CheckOnceEach},
		},
		script: func(t *testing.T, run loopRun) []string {
			once := writeSteps(t, run, findingText(run.sc, false))
			return append(append(append(readSteps(t, run), once...), once...), finalStep)
		},
	},
	{
		want: loopExpectation{
			name:     "logs_the_same_notes_twice",
			applies:  isSweep,
			absorbed: func(sc agentloope2e.Scenario) int { return len(sc.Worklist) },
		},
		script: func(t *testing.T, run loopRun) []string {
			once := writeSteps(t, run, findingText(run.sc, false))
			return append(append(append(readSteps(t, run), once...), once...), finalStep)
		},
	},
	{
		want: loopExpectation{
			name:       "logs_a_note_on_no_deal",
			applies:    isSweep,
			wantFailed: []string{agentloope2e.CheckOffList},
		},
		script: func(t *testing.T, run loopRun) []string {
			steps := append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, false))...)
			note := toolStep(t, "log_activity", integration.AnyMap{"kind": "note", "subject": "Loose end", "body": findingText(run.sc, false)})
			return append(append(steps, note), finalStep)
		},
	},
	{
		want: loopExpectation{
			name:       "annotates_an_empty_queue",
			applies:    func(sc agentloope2e.Scenario) bool { return isBrief(sc) && sc.NoWrites },
			wantFailed: []string{agentloope2e.CheckNoWrites},
		},
		script: func(t *testing.T, run loopRun) []string {
			return []string{
				toolStep(t, "read_brief", integration.AnyMap{}),
				annotation(t, run, "Nothing to report."),
				finalStep,
			}
		},
	},
	{
		want: loopExpectation{
			name:       "logs_on_a_deal_the_list_does_not_hold",
			applies:    func(sc agentloope2e.Scenario) bool { return len(unlistedDeals(sc)) > 0 && isSweep(sc) },
			wantFailed: []string{agentloope2e.CheckForbidden},
		},
		script: func(t *testing.T, run loopRun) []string {
			steps := append(readSteps(t, run), writeSteps(t, run, findingText(run.sc, false))...)
			return append(append(steps, sweepNotes(t, run, unlistedDeals(run.sc), findingText(run.sc, false))...), finalStep)
		},
	},
}

// unlistedDeals is the forbidden deals the acting member owns. The sweep's
// store refuses a note on another member's deal, so only a deal the member
// owns can reach a forbidden end state.
func unlistedDeals(sc agentloope2e.Scenario) []string {
	var owned []string
	for _, name := range sc.Forbidden {
		if deal, ok := sc.Seed.Deal(name); ok && !deal.OtherSeat {
			owned = append(owned, name)
		}
	}
	return owned
}

// failedChecks is the names of the checks a snapshot fails, once each.
func failedChecks(failures []agentloope2e.Failure) []string {
	seen := map[string]bool{}
	var names []string
	for _, failure := range failures {
		if !seen[failure.Check] {
			seen[failure.Check] = true
			names = append(names, failure.Check)
		}
	}
	slices.Sort(names)
	return names
}
