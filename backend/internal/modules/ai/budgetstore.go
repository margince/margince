// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AdminStore applies the same allowance setting as the serving policy.
// Member counting and carrier observations are injected at composition boundaries.
type AdminStore struct {
	db        *database.DB
	settings  *settings.Store
	fullUsers func(context.Context, pgx.Tx) (int64, error)
	deferred  func(context.Context) ([]crmcontracts.AiDeferredWork, error)
	now       func() time.Time
}

// NewAdminStore receives cross-module observations through composition seams.
func NewAdminStore(db *database.DB, s *settings.Store, users func(context.Context, pgx.Tx) (int64, error), deferred func(context.Context) ([]crmcontracts.AiDeferredWork, error)) *AdminStore {
	return &AdminStore{db: db, settings: s, fullUsers: users, deferred: deferred, now: time.Now}
}

// BudgetChange ties a complete draft to the configuration its editor read.
type BudgetChange struct {
	Config           BudgetConfig `json:"config"`
	ExpectedRevision string       `json:"expected_revision"`
}

func snapshotWithMonthly(config BudgetConfig, monthly, users, spent int64, now time.Time) crmcontracts.AiBudgetSnapshot {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	source := "per_user"
	if config.CompanyMonthlyTokens != nil {
		source = "company_override"
	}
	return crmcontracts.AiBudgetSnapshot{
		Config:   crmcontracts.AiBudgetConfig{TokensPerFullUser: config.TokensPerFullUser, CompanyMonthlyTokens: config.CompanyMonthlyTokens},
		Revision: config.Revision(), EligibleFullUsers: users, BudgetedFullUsers: max(users, 1), Source: source,
		MonthlyTokens: monthly, SpentTokens: spent, RemainingTokens: max(0, monthly-spent), Band: BudgetBand(spent, monthly),
		MonthStartAt: start, ResetsAt: start.AddDate(0, 1, 0), ObservedAt: now,
	}
}

func budgetSnapshot(config BudgetConfig, users, spent int64, now time.Time) (crmcontracts.AiBudgetSnapshot, error) {
	monthly, err := config.MonthlyTokens(users)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	return snapshotWithMonthly(config, monthly, users, spent, now), nil
}

// observedSnapshot never errors on a stored config that has grown past the overflow
// ceiling (see BudgetConfig.SaturatingMonthlyTokens) — it exists for surfaces that must
// render a workspace's CURRENT allowance so an admin can correct it, as opposed to
// budgetSnapshot's strict/fail-closed contract used to gate real spend and to validate a
// NEW value being written.
func observedSnapshot(config BudgetConfig, users, spent int64, now time.Time) (crmcontracts.AiBudgetSnapshot, error) {
	monthly, err := config.SaturatingMonthlyTokens(users)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	return snapshotWithMonthly(config, monthly, users, spent, now), nil
}

// loadBudgetInputs reads the pieces observedTx needs.
func (s *AdminStore) loadBudgetInputs(ctx context.Context, tx pgx.Tx) (BudgetConfig, int64, int64, time.Time, error) {
	config, err := settings.GetTx(ctx, tx, BudgetSettings)
	if err != nil {
		return BudgetConfig{}, 0, 0, time.Time{}, err
	}
	users, err := s.fullUsers(ctx, tx)
	if err != nil {
		return BudgetConfig{}, 0, 0, time.Time{}, err
	}
	now := s.now().UTC()
	spent, err := monthTokensTx(ctx, tx, now)
	if err != nil {
		return BudgetConfig{}, 0, 0, time.Time{}, err
	}
	return config, users, spent, now, nil
}

// observedTx backs this store's read surfaces (ReadStatus, PreviewRouting,
// ReadBudget, ReplaceBudget's optimistic-concurrency read, PreviewBudget's
// Current) through BudgetConfig.SaturatingMonthlyTokens, so a workspace whose
// stored config has grown past the overflow ceiling can still be read and
// corrected rather than failing every one of the entry points that exist to fix
// it. compose's seatBudget.MonthlyTokenBudget reads the same saturating method
// directly for real spend, outside this store.
func (s *AdminStore) observedTx(ctx context.Context, tx pgx.Tx) (crmcontracts.AiBudgetSnapshot, error) {
	config, users, spent, now, err := s.loadBudgetInputs(ctx, tx)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	return observedSnapshot(config, users, spent, now)
}

// ReadBudget keeps editable policy and its live calculation in one observation.
func (s *AdminStore) ReadBudget(ctx context.Context) (crmcontracts.AiBudgetSnapshot, error) {
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	var out crmcontracts.AiBudgetSnapshot
	err := s.db.Tx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.observedTx(ctx, tx); return err })
	return out, err
}

// ReplaceBudget commits policy, audit and recovery event together under a revision lock.
func (s *AdminStore) ReplaceBudget(ctx context.Context, change BudgetChange) (crmcontracts.AiBudgetSnapshot, error) {
	if err := auth.Require(ctx, budgetObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	var out crmcontracts.AiBudgetSnapshot
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, BudgetKey); err != nil {
			return err
		}
		current, err := s.observedTx(ctx, tx)
		if err != nil {
			return err
		}
		if current.Revision != change.ExpectedRevision {
			return apperrors.ErrVersionSkew
		}
		// Validate the submitted config strictly BEFORE the no-op short-circuit
		// below, so resubmitting an over-cap config unchanged is refused exactly
		// like a fresh submission would be — the observed snapshot it would
		// otherwise echo back is saturated, and reporting that as a successful
		// save would tell an admin the problem is fixed when it is not.
		out, err = previewBudget(current, change.Config)
		if err != nil {
			return err
		}
		if current.Revision == change.Config.Revision() {
			return nil
		}
		raw, err := json.Marshal(change.Config)
		if err != nil {
			return err
		}
		receipt, err := s.settings.SetRawTxReceipt(ctx, tx, BudgetKey, raw)
		if err != nil || !receipt.Changed {
			return err
		}
		return storekit.EmitEvent(ctx, tx, receipt.AuditID, storekit.MustWorkspace(ctx), crmcontracts.InternalEventAiBudgetUpdated{
			BeforeRevision: current.Revision, AfterRevision: out.Revision, TokensPerFullUser: change.Config.TokensPerFullUser, CompanyMonthlyTokens: change.Config.CompanyMonthlyTokens,
		})
	})
	return out, err
}

func previewBudget(current crmcontracts.AiBudgetSnapshot, config BudgetConfig) (crmcontracts.AiBudgetSnapshot, error) {
	out, err := budgetSnapshot(config, current.EligibleFullUsers, current.SpentTokens, current.ObservedAt)
	if err != nil {
		return out, settings.InvalidValue{Setting: BudgetKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
	}
	return out, nil
}
