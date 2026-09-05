// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
			Goal: "Prepare the acting person's existing Morning Brief. First call read_brief. " +
				"Its items are the queue already ranked for this person; do not assemble a workspace-wide list. " +
				"Read the evidence for those items, then call annotate_brief with one concise narrative " +
				"and grounded findings: why each item matters, what changed and the next move. " +
				"Use each returned item_id unchanged, never its deal_id, and cite only that item's evidence_ids. " +
				"Keep the existing order. If there are no items, finish without inventing a brief. " +
				"A tool refusal means the findings were not saved: correct it before claiming completion.",
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

// TriggerRef names one occurrence of a scheduled spec FOR ONE SEAT; the
// runner's idempotency (one run per trigger occurrence) hangs off this string.
//
// The seat belongs in the identity because the agent acts for a person. Both
// uniqueness rules that stop a double run are keyed on this ref —
// agent_run_trigger_unique on the ref alone, runner_job_trigger_unique on
// (agent_spec, ref) — so a ref naming only the spec and the day makes the
// night's work workspace-wide: whichever seat is seeded first wins the
// constraint and every other rep silently gets no run. Nothing errors, because
// one row inserting and the rest conflicting is exactly what a correct
// re-seed looks like.
//
// The seat is a DIGEST, not the user's uuid. This value is printed in the
// prompt in the runner's own voice, one line above grounding refs of the form
// `<type>:<uuid>` that name records the run actually read — so a raw id here
// gives a model a record-shaped string it may prepare against, though nothing
// was ever read to obtain it. The digest carries no such shape. It is an
// identity, never a lookup key: no reader resolves a seat from it, and the
// passport on the job row is what says who the run acts for.
func (a AgentSpec) TriggerRef(day time.Time, seat ids.UserID) string {
	return fmt.Sprintf("%s:%s:%s", a.Name, day.UTC().Format("2006-01-02"), seatDigest(seat))
}

// seatDigest is the short, stable, non-record-shaped name of one seat.
//
// WHAT A COLLISION WOULD COST is why this is not truncated further. It would be
// authority-safe — EnqueueJob conflicts to DO NOTHING, so the losing seat gets
// no job rather than inheriting the winner's passport — but it would not cost
// one night. The digest is a pure function of a stable user id, so the SAME
// seat would lose every day, for every spec it granted, silently, until
// somebody noticed one rep never gets a brief. There is no ceiling on
// installation size to bound that against, and a starvation nothing reports is
// exactly the failure this whole change exists to remove.
//
// So it keeps the full 32 hex characters. A shorter digest reads no better in
// a log line, and the only thing brevity would buy is a probability nobody has
// a reason to accept.
func seatDigest(seat ids.UserID) string {
	sum := sha256.Sum256([]byte(seat.String()))
	return hex.EncodeToString(sum[:16])
}

// DueAt is when the given day's occurrence becomes runnable.
func (a AgentSpec) DueAt(day time.Time) time.Time {
	d := day.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), a.DueHourUTC, 0, 0, 0, time.UTC)
}
