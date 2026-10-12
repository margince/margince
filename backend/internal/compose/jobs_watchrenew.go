// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mail push-subscription renewal passes, Gmail's and Microsoft Graph's:
// each walks the connections whose subscription nears expiry and renews it a
// margin ahead, the margin read from the admin's setting at every scan.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GmailWatchArgs schedules one push-watch maintenance pass: register a Gmail
// users.watch for every active connection that has none yet and renew any
// nearing its 7-day expiry (capture.md CAP-DDL-2). Scheduled only when a
// Pub/Sub topic is configured; without one, no watch job runs and capture stays
// on the poll (GmailSyncArgs).
type GmailWatchArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (GmailWatchArgs) Kind() string { return "gmail_watch_renew" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (GmailWatchArgs) FleetWide() {}

// gmailWatchWorker walks the fleet's active Gmail connections whose watch is
// missing or nearing expiry and enqueues ONE renewal job per connection,
// against the configured Pub/Sub topic. It mirrors gmailSyncWorker — the same
// collectDue-shaped walk, keyed on the renewal deadline instead of the sync
// cursor — and it fans out at the same granularity, because a watch belongs to
// a connection rather than to a workspace: a mailbox whose renewal fails is one
// connection's problem, and per-connection jobs keep it from being read as the
// tenant's.
type gmailWatchWorker struct {
	registry *capture.Registry
	pool     *pgxpool.Pool
	log      *slog.Logger
}

func (w *gmailWatchWorker) Work(ctx context.Context, _ *river.Job[GmailWatchArgs]) error {
	renewWithin, err := renewWithinOf(ctx, w.pool, identity.GmailWatchRenewWithinHours)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, dispatchDueWatches(ctx, w.registry, providerGmail, renewWithin,
		func(ws ids.UUID, connID string) error {
			return dispatchOne(ctx, GmailWatchRenewArgs{Workspace: ws, ConnectionID: connID}, nil)
		}))
}

// dispatchDueWatches is the fleet walk both providers' watch dispatchers run:
// every connection whose subscription is missing or nearing its deadline gets
// ONE renewal job. The provider and the Args type are all that differ, and a
// second copy of the walk is the copy that would stop joining the enqueue
// error.
func dispatchDueWatches(
	ctx context.Context, reg *capture.Registry, provider string,
	renewWithin time.Duration, enqueue func(ids.UUID, string) error,
) error {
	// Returns the bare cause: each caller is a River Work method, and the gate
	// that keeps a raw cause out of river_job.errors reads the return AT that
	// method — so the wrap belongs there, where a reader of the worker sees it.
	due, enumErr := reg.DueWatches(ctx, provider, renewWithin)
	for _, d := range due {
		if err := enqueue(d.Workspace.UUID, d.ID.String()); err != nil {
			// A refused enqueue means this connection's watch is never renewed,
			// so it fails the DISPATCHER rather than being logged past.
			enumErr = errors.Join(enumErr, fmt.Errorf("enqueueing the watch renewal for connection %s: %w", d.ID, err))
		}
	}
	return enumErr
}

// GmailWatchRenewArgs renews ONE connection's push watch. The workspace travels
// with it because capture_connection reads are workspace-predicated and a job
// carries no session.
type GmailWatchRenewArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID string   `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (GmailWatchRenewArgs) Kind() string { return "gmail_watch_renew_connection" }

// WorkspaceID binds this renewal to its tenant (jobs.WorkspaceScoped).
func (a GmailWatchRenewArgs) WorkspaceID() ids.UUID { return a.Workspace }

// gmailWatchRenewWorker renews one connection's watch and advances
// watch_expires_at. A revoked mailbox fails its OWN row now, where before it
// was logged and skipped inside a pass River recorded as completed.
type gmailWatchRenewWorker struct {
	registry *capture.Registry
	topic    string
}

func (w *gmailWatchRenewWorker) Work(ctx context.Context, job *river.Job[GmailWatchRenewArgs]) error {
	return jobs.FaultContext(ctx, renewOneWatch(ctx, w.registry, job.Args, job.Args.ConnectionID, w.topic))
}

// renewOneWatch is the body both providers' renewal workers run: bind the
// tenant the job carries, read the connection id it names, and let the registry
// perform the provider call and advance watch_expires_at.
//
// The topic is whatever that provider's Watch takes — a Pub/Sub topic for
// Gmail, a notification URL for Graph — and this never looks inside it.
//
// Returns the bare cause, for the reason dispatchDueWatches does.
func renewOneWatch(
	ctx context.Context, reg *capture.Registry, args jobs.WorkspaceScoped, connectionID, topic string,
) error {
	wsCtx, err := workspaceJobCtx(ctx, args)
	if err != nil {
		return err
	}
	connID, err := ids.Parse(connectionID)
	if err != nil {
		return fmt.Errorf("%s: connection id: %w", args.Kind(), err)
	}
	return reg.RenewWatch(wsCtx, connID, topic)
}

// GraphWatchArgs schedules one subscription-maintenance pass: register a Graph
// change-notification subscription for every active Outlook connection that has
// none yet, and renew any nearing its deadline. Scheduled only when a
// notification URL is configured; without one, no subscription job runs and
// Outlook capture stays on the poll.
type GraphWatchArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (GraphWatchArgs) Kind() string { return graphWatchKind }

// graphWatchKind is the kind string, named so the registration can be read from
// api/jobs.yaml without constructing the args — a dispatcher literal outside
// periodicFor is what jobdispatcherenqueue_test refuses, and rightly: building
// one to ask a question looks exactly like building one to enqueue it.
const graphWatchKind = "graph_watch_renew"

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (GraphWatchArgs) FleetWide() {}

// graphWatchWorker is the Graph twin of gmailWatchWorker, on the same walk.
type graphWatchWorker struct {
	registry *capture.Registry
	pool     *pgxpool.Pool
	log      *slog.Logger
}

func (w *graphWatchWorker) Work(ctx context.Context, _ *river.Job[GraphWatchArgs]) error {
	renewWithin, err := renewWithinOf(ctx, w.pool, identity.GraphWatchRenewWithinHours)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, dispatchDueWatches(ctx, w.registry, providerGraph, renewWithin,
		func(ws ids.UUID, connID string) error {
			return dispatchOne(ctx, GraphWatchRenewArgs{Workspace: ws, ConnectionID: connID}, nil)
		}))
}

// GraphWatchRenewArgs renews ONE connection's subscription. The workspace
// travels with it because capture_connection reads are workspace-predicated and
// a job carries no session.
type GraphWatchRenewArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID string   `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (GraphWatchRenewArgs) Kind() string { return "graph_watch_renew_connection" }

// WorkspaceID binds this renewal to its tenant (jobs.WorkspaceScoped).
func (a GraphWatchRenewArgs) WorkspaceID() ids.UUID { return a.Workspace }

// graphWatchRenewWorker renews one connection's subscription and advances
// watch_expires_at. A revoked mailbox fails its OWN row, where a pass that
// logged and skipped would be recorded as completed.
type graphWatchRenewWorker struct {
	registry        *capture.Registry
	notificationURL string
}

func (w *graphWatchRenewWorker) Work(ctx context.Context, job *river.Job[GraphWatchRenewArgs]) error {
	return jobs.FaultContext(ctx, renewOneWatch(ctx, w.registry, job.Args, job.Args.ConnectionID, w.notificationURL))
}

// renewWithinOf reads how far ahead of expiry a watch scan renews, at the
// start of the scan, so a changed margin applies to the next one.
func renewWithinOf(ctx context.Context, pool *pgxpool.Pool, entry *settings.Entry[int]) (time.Duration, error) {
	read, err := installationSettingReader(ctx, pool)
	if err != nil {
		return 0, err
	}
	raw, err := read(entry.Key())
	if err != nil {
		return 0, fmt.Errorf("compose: reading %s: %w", entry.Key(), err)
	}
	var hours int
	if err := json.Unmarshal(raw, &hours); err != nil {
		return 0, fmt.Errorf("compose: %s does not hold whole hours: %w", entry.Key(), err)
	}
	return time.Duration(hours) * time.Hour, nil
}
