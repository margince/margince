// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestACalledAmountTakesOnlyACurrencyADealWould(t *testing.T) {
	t.Parallel()
	period, err := PeriodOfDays(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkCall(NewCall{Period: period, Scope: Scope{Kind: ScopeWorkspace}, AmountMinor: 100, Currency: "EUR"}); err != nil {
		t.Fatalf("EUR refused: %v", err)
	}
	var refusal *values.ParseError
	if _, err := checkCall(NewCall{Period: period, Scope: Scope{Kind: ScopeWorkspace}, AmountMinor: 100, Currency: "ZZZ"}); !errors.As(err, &refusal) || refusal.Code != "currency_unassigned" {
		t.Fatalf("ZZZ answered %v, want the deal's currency_unassigned refusal", err)
	}
}
