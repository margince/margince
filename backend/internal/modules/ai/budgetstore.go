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

func budgetSnapshot(config BudgetConfig, users, spent int64, now time.Time) (crmcontracts.AiBudgetSnapshot, error) {
	monthly, err := config.MonthlyTokens(users)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
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
	}, nil
}

func (s *AdminStore) currentTx(ctx context.Context, tx pgx.Tx) (crmcontracts.AiBudgetSnapshot, error) {
	config, err := settings.GetTx(ctx, tx, BudgetSettings)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	users, err := s.fullUsers(ctx, tx)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	now := s.now().UTC()
	spent, err := monthTokensTx(ctx, tx, now)
	if err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	return budgetSnapshot(config, users, spent, now)
}

// ReadBudget keeps editable policy and its live calculation in one observation.
func (s *AdminStore) ReadBudget(ctx context.Context) (crmcontracts.AiBudgetSnapshot, error) {
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiBudgetSnapshot{}, err
	}
	var out crmcontracts.AiBudgetSnapshot
	err := s.db.Tx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.currentTx(ctx, tx); return err })
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
		current, err := s.currentTx(ctx, tx)
		if err != nil {
			return err
		}
		if current.Revision != change.ExpectedRevision {
			return apperrors.ErrVersionSkew
		}
		if current.Revision == change.Config.Revision() {
			out = current
			return nil
		}
		out, err = previewBudget(current, change.Config)
		if err != nil {
			return err
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
