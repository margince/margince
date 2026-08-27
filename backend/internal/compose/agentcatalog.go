// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"
	"sync"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
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
// TestTheScheduledCatalogIsTotalAgainstTheContract holds both halves total
// against each other. It does NOT hold that this is the only assembly of them —
// no test does, so this comment does not claim it. What is gated is narrower
// and is the part that matters: TestOnlySanctionedFilesBuildARunnerJob names
// the only two files that may construct a Job at all, so a second assembly
// would have nowhere to deliver its result.
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

// mustScheduledAgents reads the join once, and panics if the halves disagree.
//
// A mismatch is a build-shaped defect no deployment can configure its way out
// of — both halves are compile-time constants, and a gate holds them total
// against each other — so a panic here means the binary was assembled from
// parts that were never meant to go together.
//
// WHERE IT PANICS MATTERS, which is why NewRunnerService calls it eagerly.
// Left to first use, the first evaluation would be inside Tick on a River
// worker goroutine, and River recovers a worker panic into a job error: a
// mismatched binary would boot green and fail quietly in river_job rows, tick
// after tick. Reading it at construction turns that into a refusal to start.
//
// OnceValue also stops the join re-running per executed or resumed job.
// Harmless at two agents; it is simply not work that needs doing twice.
var mustScheduledAgents = sync.OnceValue(func() []runner.AgentSpec {
	specs, err := scheduledAgents()
	if err != nil {
		panic("compose: " + err.Error())
	}
	return specs
})

// ScheduledAgentSpecByName resolves a stored job's catalog entry WITH its
// declared allowlist. It is RunnerService's production default.
//
// Exported because WithSpecResolver leaves one door open: a caller that
// supplies its own resolver still has to answer for the agents it did NOT
// invent, and the only correct answer is this one. It is exported so that
// fallback is reachable rather than approximated — the runner's own by-name
// lookup was deleted for exactly this reason, having become a function that
// returned a spec with no allowlist and looked like the obvious choice.
func ScheduledAgentSpecByName(name string) (runner.AgentSpec, bool) {
	for _, spec := range mustScheduledAgents() {
		if spec.Name == name {
			// Clone, because the join is read ONCE for the process: every
			// caller would otherwise share one backing array, and a single
			// append or index write would change the allowlist every later job
			// runs under. A boundary that hands out its own cache is the
			// quietest way for a narrowing to stop narrowing.
			spec.Tools = slices.Clone(spec.Tools)
			return spec, true
		}
	}
	return runner.AgentSpec{}, false
}
