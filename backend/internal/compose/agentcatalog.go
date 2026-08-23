// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"

	"github.com/gradionhq/margince/backend/internal/modules/agents/runner"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
)

// The scheduled agent catalog, assembled here because neither half owns both
// pieces.
//
// runner.Catalog() knows what each agent is FOR — its goal, its hour, its
// budget. api/ai-tasks.yaml knows which tools it attaches, and it is declared
// there rather than in Go because the tool listing rides in every step of the
// window: the allowlist is simultaneously a governance boundary and a prompt
// cost, and the contract is where this repo states what an AI task costs.
//
// A module may not import a sibling (ADR-0054 §3), so runner cannot read the
// declaration and ai cannot read the catalog. Compose is where the edge is
// injected, which is what compose is for.

// scheduledAgents is the catalog every production path reads: each runner
// AgentSpec with its declared allowlist attached.
//
// It is the ONLY assembly of the two halves — RunnerService takes its default
// specByName from here and Tick seeds from here — because the alternative is
// the failure this whole change exists to remove. Job.Tools empty is read as
// NO narrowing (runner/job.go), so one production path left reading the bare
// runner.Catalog() would hand its agent every verb its passport admits, and
// nothing would look wrong: the run would work, the diff would look complete,
// and the boundary would be off.
//
// TestTheScheduledCatalogIsTotalAgainstTheContract holds the claim that this is
// the only such assembly by holding both halves total against each other.
func scheduledAgents() ([]runner.AgentSpec, error) {
	return joinScheduledAgents(runner.Catalog(), ai.AgentsFor(ai.TaskAgentLoop))
}

// joinScheduledAgents is the join itself, over its two inputs rather than over
// the package-level ones. Both refusals below are unreachable from the shipped
// tree — TestTheScheduledCatalogIsTotalAgainstTheContract is what keeps them
// so — and a branch nobody can execute is a branch nobody has tested, which is
// why the seam takes its halves as arguments.
func joinScheduledAgents(specs []runner.AgentSpec, declared []ai.Agent) ([]runner.AgentSpec, error) {
	unclaimed := make(map[string][]string, len(declared))
	for _, agent := range declared {
		unclaimed[agent.Name] = agent.Tools
	}
	assembled := make([]runner.AgentSpec, 0, len(specs))
	for _, spec := range specs {
		tools, ok := unclaimed[spec.Name]
		if !ok {
			return nil, fmt.Errorf(
				"agent %q is scheduled but the contract declares no tools for it — add it under "+
					"agent_loop's agents{} in api/ai-tasks.yaml, or the run is narrowed by its passport alone",
				spec.Name)
		}
		spec.Tools = slices.Clone(tools)
		assembled = append(assembled, spec)
		delete(unclaimed, spec.Name)
	}
	for name := range unclaimed {
		return nil, fmt.Errorf(
			"the contract declares tools for agent %q, which the runner catalog does not schedule — "+
				"an allowlist nothing runs is a rule nobody obeys", name)
	}
	return assembled, nil
}

// mustScheduledAgents is the composition-time reading. A mismatch between the
// contract and the catalog is a build-shaped defect that no deployment can
// configure its way out of, and the two are checked against each other by a
// gate, so reaching this panic means the binary was assembled from halves that
// were never meant to go together.
func mustScheduledAgents() []runner.AgentSpec {
	specs, err := scheduledAgents()
	if err != nil {
		panic("compose: " + err.Error())
	}
	return specs
}

// agentSpecByName resolves a stored job's catalog entry WITH its declared
// allowlist. It is RunnerService's production default; the seam stays open for
// the integration lane, which needs a spec no shipped agent has.
func agentSpecByName(name string) (runner.AgentSpec, bool) {
	for _, spec := range mustScheduledAgents() {
		if spec.Name == name {
			return spec, true
		}
	}
	return runner.AgentSpec{}, false
}
