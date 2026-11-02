// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	dealRefileActor       = "system:captured-deal-refile"
	dealRefileFreshWindow = 24 * time.Hour
)

// DealRefileGen acknowledges a trigger only after its resumable work is queued.
type DealRefileGen struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	now     func() time.Time
}

// NewDealRefileGen shares the normal queue and the worker's attribution engine.
func NewDealRefileGen(pool *pgxpool.Pool, enqueue *jobs.Runner, now func() time.Time) *DealRefileGen {
	return &DealRefileGen{pool: pool, enqueue: enqueue, now: now}
}

// HandleEvent persists continuation before acknowledging a trigger.
func (g *DealRefileGen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID.IsZero() {
		return nil
	}
	if g.now().Sub(env.OccurredAt) > dealRefileFreshWindow {
		slog.InfoContext(ctx, "deal refiling deferred to recovery", "event_id", env.EventID, "event_type", env.Type)
		return nil
	}
	ws, err := InstallationDB(g.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(principal.SystemActing(ctx, dealRefileActor), env.Trace.CorrelationID)
	return InstallationDB(g.pool).Tx(ctx, func(tx pgx.Tx) error {
		scope, err := dealRefileScope(ctx, tx, g.pool, env)
		if err != nil {
			return err
		}
		if scope.Empty() {
			return nil
		}
		args, err := dealRefilePassArgs(ctx, tx, scope, g.now(), env.EventID)
		if err != nil {
			return err
		}
		opts := oneOffChildOpts(args.Kind())
		opts.UniqueOpts = river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}
		return g.enqueue.EnqueueTx(ctx, tx, args, opts)
	})
}

func dealRefileScope(ctx context.Context, tx pgx.Tx, pool *pgxpool.Pool, env events.Envelope) (capture.DealRefileScope, error) {
	scope := capture.DealRefileScope{}
	switch env.Entity.Type {
	case contactObject:
		if contactGainedAddresses(env.Type) {
			scope.Contacts = []ids.UUID{env.Entity.ID}
		}
	case string(recordTypeLead):
		if env.Type != "lead.promoted" {
			return scope, nil
		}
		var promoted crmcontracts.PublicEventLeadPromoted
		if err := json.Unmarshal(env.Payload, &promoted); err != nil {
			return scope, fmt.Errorf("%w: decode %s: %v", events.ErrUnactionable, env.Type, err)
		}
		scope.Contacts = []ids.UUID{ids.UUID(promoted.PromotedContactId)}
	case string(recordTypeDeal):
		return dealAssociationRefileScope(ctx, tx, pool, env)
	case string(recordTypeCompany):
		if env.Type == "company.restored" {
			scope.Companies = []ids.UUID{env.Entity.ID}
		}
	case string(recordTypeProject):
		if env.Type == "project.restored" || env.Type == "project.updated" {
			scope.Projects = []ids.UUID{env.Entity.ID}
		}
	case string(recordTypeActivity):
		return activityRefileScope(env)
	}
	return scope, nil
}

func dealRefilePassArgs(ctx context.Context, tx pgx.Tx, scope capture.DealRefileScope, asOf time.Time, event ids.UUID) (CapturedDealRefileArgs, error) {
	ws, ok := principal.WorkspaceID(ctx)
	correlation, traced := principal.CorrelationID(ctx)
	if !ok || !traced {
		return CapturedDealRefileArgs{}, apperrors.ErrPermissionDenied
	}
	ceiling, err := capture.DealRefileCeilingTx(ctx, tx)
	return CapturedDealRefileArgs{Workspace: ws, Event: event, Correlation: correlation, Scope: scope, AsOf: asOf, Ceiling: ceiling}, err
}

func activityRefileScope(env events.Envelope) (capture.DealRefileScope, error) {
	scope := capture.DealRefileScope{}
	if env.Type != eventActivityUpdated || env.Actor.ID == cohortPromoteActor || env.Actor.ID == dealRefileActor {
		return scope, nil
	}
	var updated crmcontracts.PublicEventActivityUpdated
	if err := json.Unmarshal(env.Payload, &updated); err != nil {
		return scope, fmt.Errorf("%w: decode %s: %v", events.ErrUnactionable, env.Type, err)
	}
	if updated.ChangedFields.ThreadKey != nil && env.Actor.ID == threadRepairActor {
		scope.Activities = []ids.UUID{env.Entity.ID}
		return scope, nil
	}
	target := updated.ChangedFields.Relinked
	if target != nil && (target.EntityType == contactObject || target.EntityType == string(recordTypeCompany) || target.EntityType == string(recordTypeProject)) {
		scope.Activities = []ids.UUID{env.Entity.ID}
	}
	return scope, nil
}

func dealAssociationRefileScope(ctx context.Context, tx pgx.Tx, pool *pgxpool.Pool, env events.Envelope) (capture.DealRefileScope, error) {
	scope := capture.DealRefileScope{}
	admitted := env.Type == "deal.created" || env.Type == "deal.restored"
	if env.Type == deals.EventDealStageChanged {
		moved, err := deals.DecodeStageChanged(env.Payload)
		if err != nil {
			return scope, fmt.Errorf("%w: decode %s: %v", events.ErrUnactionable, env.Type, err)
		}
		admitted = moved.ToStatus == string(deals.DealOpen)
	}
	if env.Type == "deal.updated" {
		var updated crmcontracts.PublicEventDealUpdated
		if err := json.Unmarshal(env.Payload, &updated); err != nil {
			return scope, fmt.Errorf("%w: decode %s: %v", events.ErrUnactionable, env.Type, err)
		}
		admitted = true
	}
	if !admitted {
		return scope, nil
	}
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	companies, err := store.RefileCompaniesTx(ctx, tx, env.Entity.ID, env.Trace.AuditLogID)
	if err != nil {
		return scope, err
	}
	scope.Companies = companies
	projects, err := store.RefileProjectsTx(ctx, tx, env.Entity.ID, env.Trace.AuditLogID)
	if err != nil {
		return scope, err
	}
	scope.Projects = projects
	return scope, nil
}
