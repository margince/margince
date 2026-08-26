// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The scheduled catalog joins two halves that live in different places — the
// runner's goals and the contract's allowlists — and the join is the only thing
// standing between a declared boundary and a run bounded by its passport alone.
// These hold both directions of that join, and they hold the DEFAULT resolver
// rather than one a test supplies: a gate that checks a value the production
// path never reads proves nothing about production.

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Total in both directions. Either half alone is a half-answer: an agent the
// contract forgot runs unnarrowed, and an allowlist the catalog does not
// schedule is a rule nobody obeys — and the second is how a tool list goes on
// being maintained for an agent that was deleted a release ago.
func TestTheScheduledCatalogIsTotalAgainstTheContract(t *testing.T) {
	assembled, err := scheduledAgents()
	if err != nil {
		t.Fatalf("the runner catalog and the contract disagree: %v", err)
	}
	if len(assembled) != len(runner.Catalog()) {
		t.Fatalf("the assembly returns %d agents for a catalog of %d", len(assembled), len(runner.Catalog()))
	}
	declared := map[string][]string{}
	for _, agent := range ai.AgentsFor(ai.TaskAgentLoop) {
		declared[agent.Name] = agent.Tools
	}
	if len(declared) != len(assembled) {
		t.Errorf("the contract declares %d agents and the catalog schedules %d", len(declared), len(assembled))
	}
	for _, spec := range assembled {
		if !slices.Equal(spec.Tools, declared[spec.Name]) {
			t.Errorf("agent %q was assembled with %v, but the contract declares %v",
				spec.Name, spec.Tools, declared[spec.Name])
		}
	}
}

// The one that matters for production. RunnerService takes its specByName
// default from ScheduledAgentSpecByName, and executeJob builds the Job from whatever it
// returns — so if THIS resolver hands back a spec with no Tools, every
// scheduled run is narrowed by its passport alone and nothing looks wrong.
func TestTheDefaultResolverCarriesEachAgentsDeclaredAllowlist(t *testing.T) {
	for _, want := range runner.Catalog() {
		spec, known := ScheduledAgentSpecByName(want.Name)
		if !known {
			t.Errorf("the default resolver does not know scheduled agent %q", want.Name)
			continue
		}
		if len(spec.Tools) == 0 {
			t.Errorf("the default resolver returns agent %q with no tools — an empty allowlist is read "+
				"as NO narrowing, so this run would be bounded by its passport alone", want.Name)
		}
	}
	if _, known := ScheduledAgentSpecByName("an_agent_no_release_ever_shipped"); known {
		t.Error("the default resolver claims to know an agent the catalog does not schedule")
	}
}

// The join is read once for the process, so what the resolver hands out must
// not be the cache itself: one caller appending to its own allowlist would
// otherwise widen — or reorder — the allowlist every later job runs under, and
// nothing about the call site would look wrong.
func TestTheResolverHandsOutItsOwnCopyOfTheAllowlist(t *testing.T) {
	const agent = "morning_brief"
	first, known := ScheduledAgentSpecByName(agent)
	if !known {
		t.Fatalf("%s is not scheduled", agent)
	}
	if len(first.Tools) == 0 {
		t.Fatalf("%s resolves with no tools", agent)
	}
	before := slices.Clone(first.Tools)
	first.Tools[0] = "a_tool_a_caller_wrote_over_the_cache"
	first.Tools = append(first.Tools, "and_one_it_appended")

	second, _ := ScheduledAgentSpecByName(agent)
	if !slices.Equal(second.Tools, before) {
		t.Errorf("after one caller mutated its own copy, the next resolution returned %v instead of %v — "+
			"the resolver is handing out the process-wide cache, so a narrowing can be widened by accident",
			second.Tools, before)
	}
}

// The assembly's own refusals, proved against inputs that break them. Neither
// is reachable from the shipped tree — that is the point of the gate above —
// so they would otherwise be branches nobody has ever seen execute.
func TestTheAssemblyRefusesEitherHalfGoingMissing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		specs    []runner.AgentSpec
		declared []ai.Agent
		wantErr  string
	}{
		{
			name:     "an agent the contract forgot",
			specs:    []runner.AgentSpec{{Name: "morning_brief"}},
			declared: nil,
			wantErr:  "declares no tools for it",
		},
		{
			name:     "an allowlist nothing schedules",
			specs:    nil,
			declared: []ai.Agent{{Name: "a_retired_agent", Tools: []string{"read_record"}}},
			wantErr:  "does not schedule",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := joinScheduledAgents(tc.specs, tc.declared)
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("the refusal does not say what is wrong: want it to mention %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// The other half of runner's TestNoUnattendedAgentSpecCanAnswerAnApproval,
// here because the allowlist is here.
//
// decide_approval and decide_approval_bundle are auto-execute and write-scoped,
// so nothing in the admission gate stops a run calling them: a passport that
// may decide is exactly what an interactive caller needs, and it is the RUN
// that must not be able to spend it. The declaration is the only thing in the
// way, and a decide verb added to it would look like an ordinary line in a
// list of tools — while making a scheduled run able to release the calls it
// staged for itself, which is the first unattended self-approval in the tree.
func TestNoScheduledAgentAttachesADecideVerb(t *testing.T) {
	decideVerbs := []string{"decide_approval", "decide_approval_bundle"}
	agents := mustScheduledAgents()
	if len(agents) == 0 {
		t.Fatal("no scheduled agents — this gate checked nothing")
	}
	checked := 0
	for _, spec := range agents {
		for _, tool := range spec.Tools {
			checked++
			if slices.Contains(decideVerbs, tool) {
				t.Errorf("agent %q attaches %s, so a scheduled run could release the calls it stages "+
					"for itself — the confirm-first tier is a formality for that agent", spec.Name, tool)
			}
		}
	}
	if checked == 0 {
		t.Error("no agent attaches any tool, so this gate compared nothing — the declaration is not " +
			"reaching the assembled catalog")
	}
}
