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

	"github.com/gradionhq/margince/backend/internal/modules/agents/runner"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
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
// default from agentSpecByName, and executeJob builds the Job from whatever it
// returns — so if THIS resolver hands back a spec with no Tools, every
// scheduled run is narrowed by its passport alone and nothing looks wrong.
func TestTheDefaultResolverCarriesEachAgentsDeclaredAllowlist(t *testing.T) {
	for _, want := range runner.Catalog() {
		spec, known := agentSpecByName(want.Name)
		if !known {
			t.Errorf("the default resolver does not know scheduled agent %q", want.Name)
			continue
		}
		if len(spec.Tools) == 0 {
			t.Errorf("the default resolver returns agent %q with no tools — an empty allowlist is read "+
				"as NO narrowing, so this run would be bounded by its passport alone", want.Name)
		}
	}
	if _, known := agentSpecByName("an_agent_no_release_ever_shipped"); known {
		t.Error("the default resolver claims to know an agent the catalog does not schedule")
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
