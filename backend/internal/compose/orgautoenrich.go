// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The prompt half of captured-organization auto-enrich: an organization
// event queues the workspace's enrich pass NOW.
//
// The daily sweep (captureautoenrich.go) is the reconciler, and before this
// consumer it was also the only trigger for a company a person or an agent
// CREATED — so one minted five minutes after the sweep waited a day for its
// dossier, which is exactly the moment its creator is looking at the empty
// page. THE TRIGGER IS THE EVENT, NOT THE WRITER (the person-auto-enrich
// rule): organization.created and organization.updated reach the outbox
// because the write shape puts them there, so manual entry, the MCP tools,
// a site-read confirm and an import all land here without any of them
// knowing this consumer exists.
//
// It queues the WORKSPACE PASS, not a read for the event's organization.
// The pass already owns every gate — the auto-enrich setting, the daily
// budget, the cursor backoff, the dossier check — and re-derives which
// organizations are due, so this consumer cannot become a second spelling
// of that decision, and a burst of creates costs redundant no-op passes
// rather than redundant crawls.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/platform/jobs"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/events"
)

// OrgAutoEnrichTrigger queues one auto-enrich workspace pass per
// organization event.
type OrgAutoEnrichTrigger struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	log     *slog.Logger
}

// NewOrgAutoEnrichTrigger builds the trigger over an insert-only jobs runner.
func NewOrgAutoEnrichTrigger(pool *pgxpool.Pool, enqueue *jobs.Runner, log *slog.Logger) *OrgAutoEnrichTrigger {
	return &OrgAutoEnrichTrigger{pool: pool, enqueue: enqueue, log: log}
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic. An enqueue failure comes back as an error and the bus
// redelivers — safe, because a redelivered event only queues another no-op
// pass, and the sweep still reconciles whatever slips through.
func (g *OrgAutoEnrichTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	switch env.Type {
	// Every event that can make an organization newly due: appearing,
	// gaining a domain (a company save's domain change arrives as
	// organization.updated), or inheriting one in a merge. An archive needs
	// no reaction — the pass only ever starts reads, and an archived
	// organization is not due.
	case "organization.created", "organization.updated", "organization.merged":
	default:
		return nil
	}
	// The envelope carries no tenant (ADR-0091 §6); the store's handle names it.
	ws, err := InstallationDB(g.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	// One pass per event, no uniqueness — per oneOffChildOpts' own warning: a
	// pass already RUNNING may have read the workspace before this event's
	// rows landed, and River refuses a unique-state list that exempts
	// running, so any dedupe here could drop exactly the organization the
	// event fired about. The redundant passes a burst produces are a few
	// SELECTs each: every gate they re-check — the cursor, the in-flight
	// dossier index, the budget reservation — already makes a second pass
	// over the same organization a no-op.
	child := CaptureAutoEnrichWorkspaceArgs{Workspace: ws.UUID}
	return g.enqueue.Enqueue(ctx, child, oneOffChildOpts(child.Kind()))
}
