// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// BudgetKey names the installation setting shared by runtime and administration.
const (
	BudgetKey    = "ai.budget"
	budgetObject = "ai_budget"
)

// MaxMonthlyTokens bounds both saved inputs and multiplication after membership changes.
const MaxMonthlyTokens int64 = 1_000_000_000_000

// BudgetConfig is a company pool, not a quota imposed on each member.
type BudgetConfig struct {
	TokensPerFullUser    int64  `json:"tokens_per_full_user"`
	CompanyMonthlyTokens *int64 `json:"company_monthly_tokens"`
}

// UnmarshalJSON distinguishes an explicit automatic allowance from an omitted field.
func (c *BudgetConfig) UnmarshalJSON(raw []byte) error {
	type document struct {
		TokensPerFullUser    *int64          `json:"tokens_per_full_user"`
		CompanyMonthlyTokens json.RawMessage `json:"company_monthly_tokens"`
	}
	var value document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value.TokensPerFullUser == nil || len(value.CompanyMonthlyTokens) == 0 {
		return fmt.Errorf("both allowance fields are required")
	}
	var company *int64
	if err := json.Unmarshal(value.CompanyMonthlyTokens, &company); err != nil {
		return err
	}
	*c = BudgetConfig{TokensPerFullUser: *value.TokensPerFullUser, CompanyMonthlyTokens: company}
	return nil
}

// BudgetSettings preserves the existing default and survives data resets.
var BudgetSettings = settings.Define(BudgetKey, budgetObject, "update",
	BudgetConfig{TokensPerFullUser: int64(DefaultMonthlyTokens)}, validateBudget).
	MachineryApplied().AsInstallationIdentity()

func validateBudget(c BudgetConfig) error {
	if c.TokensPerFullUser < 1 || c.TokensPerFullUser > MaxMonthlyTokens {
		return fmt.Errorf("tokens per full user must be between 1 and %d", MaxMonthlyTokens)
	}
	if c.CompanyMonthlyTokens != nil && (*c.CompanyMonthlyTokens < 1 || *c.CompanyMonthlyTokens > MaxMonthlyTokens) {
		return fmt.Errorf("company allowance must be between 1 and %d", MaxMonthlyTokens)
	}
	return nil
}

// resolveMonthlyTokens applies the override or the live full-user count with an
// onboarding floor, delegating the overflow decision to the caller. MonthlyTokens
// rejects it, because a NEW value being written must never exceed the ceiling.
// SaturatingMonthlyTokens clamps it instead, because every other caller reads an
// ALREADY-STORED config (spend, diagnostics, admin observation) and a clamp can
// only ever authorize less than what was configured — never more — so it is the
// conservative choice for a value nobody is choosing right now.
func (c BudgetConfig) resolveMonthlyTokens(fullUsers int64, onOverflow func() (int64, error)) (int64, error) {
	if err := validateBudget(c); err != nil {
		return 0, err
	}
	if c.CompanyMonthlyTokens != nil {
		return *c.CompanyMonthlyTokens, nil
	}
	users := max(fullUsers, 1)
	if users > MaxMonthlyTokens/c.TokensPerFullUser {
		return onOverflow()
	}
	return users * c.TokensPerFullUser, nil
}

// MonthlyTokens validates a value being WRITTEN: nothing may be saved past the
// ceiling. Every reader of an already-stored config uses SaturatingMonthlyTokens
// instead — this stays strict only for the write path (previewBudget/ReplaceBudget).
func (c BudgetConfig) MonthlyTokens(fullUsers int64) (int64, error) {
	return c.resolveMonthlyTokens(fullUsers, func() (int64, error) {
		return 0, fmt.Errorf("company allowance exceeds the supported maximum: lower tokens per full user, or set a company-wide monthly override")
	})
}

// SaturatingMonthlyTokens reads an already-stored config: a product that has grown
// past MaxMonthlyTokens since the value was written saturates at the ceiling
// instead of erroring, so every reader — real spend (compose's
// seatBudget.MonthlyTokenBudget), the resume sweep, usage reporting, and admin
// observation alike — keeps working against the ceiling rather than halting until
// an admin corrects a config nothing here is choosing anew. A NEW value being
// written is still rejected outright by MonthlyTokens, so the ceiling itself is
// never weakened by this path.
func (c BudgetConfig) SaturatingMonthlyTokens(fullUsers int64) (int64, error) {
	return c.resolveMonthlyTokens(fullUsers, func() (int64, error) { return MaxMonthlyTokens, nil })
}

// Revision hashes the canonical typed document without mixing in live consumption.
func (c BudgetConfig) Revision() string {
	// The fixed scalar shape cannot fail to encode.
	company := "null"
	if c.CompanyMonthlyTokens != nil {
		company = fmt.Sprint(*c.CompanyMonthlyTokens)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf(`{"tokens_per_full_user":%d,"company_monthly_tokens":%s}`, c.TokensPerFullUser, company))))
}
