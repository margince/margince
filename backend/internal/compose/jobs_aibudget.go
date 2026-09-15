// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AIBudgetResumeArgs reconciles saved deferrals against the current allowance.
type AIBudgetResumeArgs struct{}

// Kind is the persisted queue identifier.
func (AIBudgetResumeArgs) Kind() string { return "ai_budget_resume" }

// FleetWide lets one periodic pass reconcile each installation workspace.
func (AIBudgetResumeArgs) FleetWide() {}

// InsertOpts leaves reconciliation failures visible without a long River backoff.
// The next minute's tick is the retry; uniqueness covers active passes only.
func (AIBudgetResumeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByState: activeSweepStates}}
}

type aiBudgetResumeWorker struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	inserter func() (*jobs.Runner, error)
}

func (w *aiBudgetResumeWorker) Work(ctx context.Context, _ *river.Job[AIBudgetResumeArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.resumeWorkspace))
}

func (w *aiBudgetResumeWorker) resumeWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithWorkspaceID(ctx, workspace)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:ai-budget-resume"})
	db := InstallationDB(w.pool)
	total, err := NewSeatBudget(w.pool).MonthlyTokenBudget(ctx, ids.From[ids.WorkspaceKind](workspace))
	if err != nil {
		return err
	}
	spent, err := ai.NewMeter(db).MonthTokens(ctx)
	if err != nil {
		return err
	}
	if ai.BudgetBand(spent, total) == ai.BandQueued {
		return nil
	}
	runner, err := w.inserter()
	if err != nil {
		return err
	}
	users := identity.NewService(w.pool)
	contactStore := contacts.NewStore(db)
	siteErr := contactStore.ResumeBudgetReads(ctx, func(ctx context.Context, tx pgx.Tx, read contacts.BudgetRead) (bool, error) {
		if err := resumeSiteReadAuthority(ctx, tx, users, read); err != nil {
			return false, err
		}
		return runner.ResumeScheduledTx(ctx, tx, "site_deep_read", "site_read_id", read.ID, workspace)
	})
	scanErr := companyscan.ResumeBudgetScans(ctx, db, func(ctx context.Context, tx pgx.Tx, id, requester, company ids.UUID) (bool, error) {
		if err := resumeCompanyAuthority(ctx, tx, users, requester, id, company); err != nil {
			return false, err
		}
		return runner.ResumeScheduledTx(ctx, tx, "account_scan", "scan_id", id, workspace)
	})
	voice := ai.NewVoiceStore(db)
	voiceErr := voice.ResumeBudgetBuilds(ctx, func(ctx context.Context, tx pgx.Tx, build ai.VoiceBuild) (bool, error) {
		if build.RequestedBy == nil {
			return false, apperrors.ErrNotFound
		}
		userCtx, err := companyscan.WorkerContext(ctx, users, ids.From[ids.UserKind](*build.RequestedBy), build.ID)
		if err != nil {
			return false, err
		}
		if err := auth.Require(userCtx, "voice_profile", principal.ActionUpdate); err != nil {
			return false, err
		}
		if _, err := voice.GetProfile(userCtx, build.ProfileID); err != nil {
			return false, err
		}
		inserted, err := runner.EnqueueTxUnique(ctx, tx, VoiceBuildArgs{Workspace: workspace, ProfileID: build.ProfileID.String(), BuildID: build.ID.String(), RequestedBy: build.RequestedBy.String()}, voiceBuildInsertOpts())
		if err != nil || inserted {
			return inserted, err
		}
		return runner.ResumeScheduledTx(ctx, tx, "voice_build", "build_id", build.ID, workspace)
	})
	return errors.Join(siteErr, scanErr, voiceErr)
}

func resumeCompanyAuthority(ctx context.Context, tx pgx.Tx, users *identity.Service, requester, id, company ids.UUID) error {
	userCtx, err := companyscan.WorkerContext(ctx, users, ids.From[ids.UserKind](requester), id)
	if err != nil {
		return err
	}
	if err := auth.Require(userCtx, "company", principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureVisible(userCtx, tx, "company", company)
}

func resumeSiteReadAuthority(ctx context.Context, tx pgx.Tx, users *identity.Service, read contacts.BudgetRead) error {
	requesterCtx := ctx
	human, ok := principal.HumanUserID(read.Requester)
	if ok {
		var err error
		requesterCtx, err = companyscan.WorkerContext(ctx, users, ids.From[ids.UserKind](human), read.ID)
		if err != nil {
			return err
		}
	} else if !isSystemRead(read.Requester) {
		// isSystemRead is the single place a lane declares itself automatic; an
		// automatic lane's requester resumes here on the same predicate that
		// already gates its page ceiling, so a new lane only ever needs one
		// declaration to resume correctly too.
		return fmt.Errorf("site read %s: requester cannot be resolved", read.ID)
	}
	return contacts.RequireSiteReadAuthority(requesterCtx, tx, read.CompanyID, read.TargetKind)
}

func newAIBudgetResumeWorker(pool *pgxpool.Pool, log *slog.Logger) *aiBudgetResumeWorker {
	return &aiBudgetResumeWorker{pool: pool, log: log, inserter: sync.OnceValues(func() (*jobs.Runner, error) {
		return jobs.NewInserter(pool, log)
	})}
}
