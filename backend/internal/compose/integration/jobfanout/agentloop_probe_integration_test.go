// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
)

// probed is what the real tools answered, by seed name. A model reads these ids
// and no others, so a seed whose records the tools cannot reach fails the probe.
type probed struct {
	deals    map[string]string
	items    map[string]string
	evidence map[string][]string
	touches  map[string][]string
}

// dealID is the id the tools served for a deal. For a deal no tool served, such
// as a forbidden one, it is the id the seed minted.
func (p *probed) dealID(w *loopWorld, name string) string {
	if id, ok := p.deals[name]; ok {
		return id
	}
	return w.deals[name]
}

// probeSeed runs the agent once on the seeded workspace with reads only, before
// any model under test. It holds the seed to what the scenario relies on. The
// run is the worker's own, so the answers are the ones a model would get. A
// failure is a harness error, so no paid run starts on a broken seed.
func (re *runnerEnv) probeSeed(t *testing.T, sc agentloope2e.Scenario, w *loopWorld) *probed {
	t.Helper()
	trigger := string(sc.Agent) + ":e2e-probe"
	re.brain.Script(append(probeSteps(t, sc, w), finalStep)...)
	re.enqueue(t, string(sc.Agent), trigger, &re.passportID)
	re.tick(t)
	status, trace, _ := re.runRow(t, trigger)
	if status != string(runner.OutcomeCompleted) {
		t.Fatalf("harness: the probe run ended %s", status)
	}
	p := &probed{
		deals: map[string]string{}, items: map[string]string{},
		evidence: map[string][]string{}, touches: map[string][]string{},
	}
	var planted bool
	for _, step := range trace {
		if step.Admission != "executed" {
			t.Fatalf("harness: the probe's %s was %s: %s", step.Tool, step.Admission, step.Observation)
		}
		p.learn(t, w, step)
		planted = planted || (step.Tool == "read_record" && sc.Canary != "" && strings.Contains(step.Observation, sc.Canary))
	}
	if sc.Canary != "" && !planted {
		t.Fatalf("harness: no real read returned the planted text %q", sc.Canary)
	}
	p.assertReaches(t, sc, w)
	re.assertGrounded(t, sc)
	return p
}

func probeSteps(t *testing.T, sc agentloope2e.Scenario, w *loopWorld) []string {
	t.Helper()
	steps := []string{toolStep(t, "read_brief", integration.AnyMap{})}
	if sc.Agent == agentloope2e.OvernightSweep {
		steps = []string{toolStep(t, "whats_slipping_this_week", integration.AnyMap{})}
	}
	for _, name := range sc.Worklist {
		steps = append(steps, toolStep(t, "catch_me_up_on", integration.AnyMap{"record_type": "deal", "record_id": w.deals[name]}))
		for _, activity := range w.activities[name] {
			steps = append(steps, toolStep(t, "read_record", integration.AnyMap{"record_type": "activity", "id": activity}))
		}
	}
	return steps
}

// The shapes of the answers a probe reads. Each tool wraps its answer in data.
type probeAnswer struct {
	Data struct {
		Items []struct {
			ItemID   string   `json:"item_id"`
			DealID   string   `json:"deal_id"`
			Evidence []string `json:"evidence_ids"`
		} `json:"items"`
		Deals []struct {
			DealID string `json:"deal_id"`
		} `json:"deals"`
		Anchor struct {
			RecordID string `json:"record_id"`
		} `json:"anchor"`
		Sections []struct {
			Name  string `json:"name"`
			Items []struct {
				RecordType string `json:"record_type"`
				RecordID   string `json:"record_id"`
			} `json:"items"`
		} `json:"sections"`
	} `json:"data"`
}

// learn files what one probe step's answer says under the seed's names.
func (p *probed) learn(t *testing.T, w *loopWorld, step runner.Step) {
	t.Helper()
	var answer probeAnswer
	if err := json.Unmarshal([]byte(step.Observation), &answer); err != nil && step.Tool != "read_record" {
		t.Fatalf("harness: the probe's %s answered something unreadable: %v", step.Tool, err)
	}
	for _, item := range answer.Data.Items {
		if name := w.nameOf(item.DealID); name != "" {
			p.deals[name], p.items[name], p.evidence[name] = item.DealID, item.ItemID, item.Evidence
		}
	}
	for _, deal := range answer.Data.Deals {
		if name := w.nameOf(deal.DealID); name != "" {
			p.deals[name] = deal.DealID
		}
	}
	name := w.nameOf(answer.Data.Anchor.RecordID)
	for _, section := range answer.Data.Sections {
		for _, item := range section.Items {
			if name != "" && section.Name == "recent_touches" && item.RecordType == "activity" {
				p.touches[name] = append(p.touches[name], item.RecordID)
			}
		}
	}
}

// assertReaches holds the tools' own answers to the scenario. The queue or list
// holds each worklist deal, never a forbidden one, and the answer for a deal
// carries every activity the seed gave it. For the brief that is its evidence,
// which is only what happened since the rep's last view. For the sweep it is the
// touches catch_me_up_on lists.
func (p *probed) assertReaches(t *testing.T, sc agentloope2e.Scenario, w *loopWorld) {
	t.Helper()
	for _, name := range sc.Worklist {
		if p.deals[name] == "" {
			t.Fatalf("harness: the tools never served %s, so the agent cannot work it", name)
		}
		reached := p.touches[name]
		if sc.Agent == agentloope2e.MorningBrief {
			reached = p.evidence[name]
		}
		for _, activity := range w.activities[name] {
			if !slices.Contains(reached, activity) {
				t.Fatalf("harness: the seeded activity %s of %s is not in what the tools served for it", activity, name)
			}
		}
	}
	for _, name := range sc.Forbidden {
		if _, served := p.deals[name]; served {
			t.Fatalf("harness: the tools served %s, which the run must never touch", name)
		}
	}
	if sc.Agent == agentloope2e.MorningBrief && len(p.items) != len(sc.Worklist) {
		t.Fatalf("harness: the brief served %d items for %d worklist deals", len(p.items), len(sc.Worklist))
	}
}

// assertGrounded holds the seed's forbidden deals to the retrieval the run is
// seeded with. The case is a model told about a deal that is not its queue, so
// the first model request must carry that deal's activity text.
func (re *runnerEnv) assertGrounded(t *testing.T, sc agentloope2e.Scenario) {
	t.Helper()
	var prompt string
	for _, call := range re.brain.Calls() {
		if call.Op == "complete" {
			prompt = string(call.Payload)
			break
		}
	}
	for _, name := range sc.Forbidden {
		deal, _ := sc.Seed.Deal(name)
		for _, activity := range deal.Activities {
			if !strings.Contains(prompt, activity.Body) {
				t.Fatalf("harness: grounding did not put the text of %s in the first model request", name)
			}
		}
	}
}
