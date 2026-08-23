// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"fmt"
	"time"
)

// AgentSpec is one catalog entry: a named, scheduled, budgeted goal.
// The catalog is code, not configuration — adding an agent is a
// reviewed change, exactly like adding a workflow handler.
type AgentSpec struct {
	Name string
	Goal string
	// DueHourUTC is the daily trigger hour. Workspace-local scheduling
	// (each tenant's own 06:00) needs the workspace timezone plumbed
	// through the seeder; V1 runs the catalog on UTC.
	DueHourUTC int
	Budget     Budget
	// Tools is what this goal may call, and the SCOPE MODEL is why it has
	// to exist. A passport carries scopes, and `write` is all-or-nothing
	// across twelve verbs — so an agent that needs `log_activity` is
	// handed `archive_record`, `merge_records` and `update_record` with
	// it. Neither the passport nor the admission gate can say "this one
	// write and no other"; only the catalog entry can, because only it
	// knows what the goal is for.
	//
	// It NARROWS and never grants. Every call still passes the same gate
	// against the same passport, so this is a second lock and not a
	// second key — one registry, one audit stream, agent ≤ human
	// (ADR-0009 Decision 5). A name here the passport does not admit
	// stays refused.
	//
	// Required and non-empty: an empty set is read as "no narrowing" at the
	// Job seam, so a spec that loses its list quietly regains the whole
	// catalog — and a misspelt verb is how an agent silently loses the one
	// tool its goal depends on.
	//
	// IT IS NOT FILLED IN HERE. The value is declared in api/ai-tasks.yaml
	// under agent_loop's agents{} and attached by compose, because the
	// listing rides in every step of the window: the allowlist is a prompt
	// COST as well as a boundary, and the contract is where this repo says
	// what an AI task costs. A module may not import a sibling, so this
	// package cannot read the declaration itself. Read a spec through
	// compose's scheduledAgents(), never through Catalog() directly.
	Tools []string
}

// Catalog is the V1 agent set (B-EP06.22): the Morning Brief and the
// overnight at-risk sweep — the two judgment tasks Surface A and the
// deterministic workflow path structurally cannot do.
//
// The entries carry NO Tools: that half is declared in api/ai-tasks.yaml and
// joined on by compose's scheduledAgents(). A caller that ranges this directly
// and builds a Job from it produces a run narrowed by its passport alone —
// which is why no production path does.
func Catalog() []AgentSpec {
	return []AgentSpec{
		{
			Name: "morning_brief",
			Goal: "Assemble the Morning Brief for this workspace: enumerate open deals, " +
				"read the ones with recent activity, and produce a ranked list (at most 7) of " +
				"deals the team can win this week. For each: why it is on the list, what changed " +
				"recently, and one recommended next move — every claim grounded in a record you " +
				"actually read, citing its id. A quiet day yields a short list; never pad it.",
			DueHourUTC: 6,
		},
		{
			Name: "overnight_at_risk_sweep",
			Goal: "Sweep this workspace's open deals for risk: find deals with no activity in " +
				"14+ days, stakeholders gone quiet, or missing next steps. Log ONE note activity " +
				"per at-risk deal summarizing the risk and the evidence (cite the records you " +
				"read). Do not advance stages, send anything, or archive anything.",
			DueHourUTC: 2,
		},
	}
}

// TriggerRef names one occurrence of a scheduled spec; the runner's
// idempotency (one run per trigger occurrence) hangs off this string.
func (a AgentSpec) TriggerRef(day time.Time) string {
	return fmt.Sprintf("%s:%s", a.Name, day.UTC().Format("2006-01-02"))
}

// DueAt is when the given day's occurrence becomes runnable.
func (a AgentSpec) DueAt(day time.Time) time.Time {
	d := day.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), a.DueHourUTC, 0, 0, 0, time.UTC)
}
