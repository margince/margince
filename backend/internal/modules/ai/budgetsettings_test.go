// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAllowanceDefaultsAndBounds(t *testing.T) {
	raw, err := BudgetSettings.DefaultJSON()
	if err != nil {
		t.Fatal(err)
	}
	var config BudgetConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.TokensPerFullUser != int64(DefaultMonthlyTokens) || !BudgetSettings.SurvivesDataReset() {
		t.Fatal("default or reset policy drifted")
	}
	for _, users := range []int64{0, 1, 4} {
		got, err := config.MonthlyTokens(users)
		if err != nil || got != max(1, users)*int64(DefaultMonthlyTokens) {
			t.Fatalf("users %d: %d %v", users, got, err)
		}
	}
	limit := int64(500)
	config.CompanyMonthlyTokens = &limit
	if got, err := config.MonthlyTokens(4); err != nil || got != limit {
		t.Fatalf("override: %d %v", got, err)
	}
	config.CompanyMonthlyTokens = nil
	config.TokensPerFullUser = MaxMonthlyTokens
	if _, err := config.MonthlyTokens(2); err == nil {
		t.Fatal("overflow accepted")
	}
	for _, value := range []int64{-1, 0, MaxMonthlyTokens + 1} {
		config.TokensPerFullUser = value
		if _, err := config.MonthlyTokens(1); err == nil {
			t.Fatalf("accepted %d", value)
		}
	}
}

func TestAllowanceSaturatesInsteadOfErroringForAnAlreadyStoredOverflow(t *testing.T) {
	config := BudgetConfig{TokensPerFullUser: MaxMonthlyTokens}
	// A NEW value overflowing against the current count is still rejected outright —
	// the ceiling on WRITES is unweakened.
	if _, err := config.MonthlyTokens(2); err == nil {
		t.Fatal("MonthlyTokens must still reject an overflowing product")
	}
	// The same overflow, OBSERVED rather than written, saturates instead.
	got, err := config.SaturatingMonthlyTokens(2)
	if err != nil || got != MaxMonthlyTokens {
		t.Fatalf("saturating observation: %d %v", got, err)
	}
	// Within bounds, both report the identical value.
	inBounds := BudgetConfig{TokensPerFullUser: 100}
	strict, err := inBounds.MonthlyTokens(3)
	if err != nil {
		t.Fatal(err)
	}
	saturating, err := inBounds.SaturatingMonthlyTokens(3)
	if err != nil || saturating != strict {
		t.Fatalf("in-bounds divergence: strict=%d saturating=%d err=%v", strict, saturating, err)
	}
	// A CompanyMonthlyTokens override never overflows via multiplication, so both
	// report the override untouched.
	limit := int64(500)
	overridden := BudgetConfig{TokensPerFullUser: 1, CompanyMonthlyTokens: &limit}
	if got, err := overridden.SaturatingMonthlyTokens(9); err != nil || got != limit {
		t.Fatalf("override: %d %v", got, err)
	}
	// A config that fails validateBudget itself (not merely a growth overflow) is
	// still an error under both — saturating only covers the overflow case.
	invalid := BudgetConfig{TokensPerFullUser: -1}
	if _, err := invalid.SaturatingMonthlyTokens(1); err == nil {
		t.Fatal("an out-of-range stored value must still error")
	}
}

func TestAllowanceRequiresAnExplicitIntegerShape(t *testing.T) {
	for _, raw := range []string{`{}`, `{"tokens_per_full_user":12}`, `{"tokens_per_full_user":1.5,"company_monthly_tokens":null}`, `{"tokens_per_full_user":12,"company_monthly_tokens":null,"extra":1}`, `{"tokens_per_full_user":null,"company_monthly_tokens":null}`} {
		var config BudgetConfig
		if err := json.Unmarshal([]byte(raw), &config); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	var a, b BudgetConfig
	for raw, target := range map[string]*BudgetConfig{`{"tokens_per_full_user":12,"company_monthly_tokens":null}`: &a, `{"company_monthly_tokens":null,"tokens_per_full_user":12}`: &b} {
		if err := json.Unmarshal([]byte(raw), target); err != nil {
			t.Fatal(err)
		}
	}
	if a.Revision() != b.Revision() {
		t.Fatal("JSON ordering changed the revision")
	}
}

func TestObservedSnapshotSaturatesWhereBudgetSnapshotErrors(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	config := BudgetConfig{TokensPerFullUser: MaxMonthlyTokens}
	if _, err := budgetSnapshot(config, 2, 0, now); err == nil {
		t.Fatal("budgetSnapshot must still fail closed on an overflowing product")
	}
	observed, err := observedSnapshot(config, 2, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if observed.MonthlyTokens != MaxMonthlyTokens || observed.Revision != config.Revision() || observed.EligibleFullUsers != 2 {
		t.Fatalf("observed snapshot %+v", observed)
	}
}

func TestAllowanceSnapshotIsUTCAndDoesNotHideOverspend(t *testing.T) {
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.FixedZone("east", 7*3600))
	snapshot, err := budgetSnapshot(BudgetConfig{TokensPerFullUser: 100}, 0, 106, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.EligibleFullUsers != 0 || snapshot.BudgetedFullUsers != 1 || snapshot.RemainingTokens != 0 || snapshot.SpentTokens != 106 || snapshot.Band != BandQueued {
		t.Fatalf("snapshot %+v", snapshot)
	}
	if snapshot.ResetsAt != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("reset %s", snapshot.ResetsAt)
	}
}

func TestAllowanceGatesBeforeReadingOrReturningAConflict(t *testing.T) {
	store := NewAdminStore(nil, nil, nil, nil)
	ctx := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test", SeatType: principal.SeatFull})
	if _, err := store.ReadBudget(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read: %v", err)
	}
	if _, err := store.ReplaceBudget(ctx, BudgetChange{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("save: %v", err)
	}
	if _, err := store.ReadStatus(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("status: %v", err)
	}
	if _, err := store.PreviewRouting(ctx, RoutingConfig{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("preview: %v", err)
	}
}
