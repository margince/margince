// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A masked reader's claim and observed slots are withheld only on the types
// that quote money, so a date-quoting finding keeps its slots. The type list is
// bound once and shared by both slots.
func TestAFindingsMoneySlotsAreMaskedOnlyOnTheTypesThatQuoteMoney(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:masked-reader",
		Permissions: principal.Permissions{
			Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
		},
	})
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }

	affected, err := auth.MaskedExpressionSQL(ctx, tableDeal, "amount_minor", "d", rawFindingMoney.affected, arg)
	if err != nil {
		t.Fatal(err)
	}
	money, err := maskedFindingMoney(ctx, affected, arg)
	if err != nil {
		t.Fatal(err)
	}

	if money.affected == rawFindingMoney.affected {
		t.Fatalf("affected = %q, want the masked rendering", money.affected)
	}
	typeLists := 0
	for _, value := range args {
		if types, ok := value.([]string); ok && slices.Equal(types, assurance.DealAmountTypes()) {
			typeLists++
		}
	}
	if typeLists != 1 {
		t.Errorf("the money-quoting types were bound %d times, want once for both slots", typeLists)
	}
	for slot, rendered := range map[string]string{"claim": money.claim, "observed": money.observed} {
		raw := "e." + slot
		if !strings.Contains(rendered, "CASE WHEN e.type = ANY(") || !strings.HasSuffix(rendered, "ELSE "+raw+" END") {
			t.Errorf("%s = %q, want it masked on the money-quoting types and %s on every other", slot, rendered, raw)
		}
	}
}

// A finding's currency answers to the deal's currency mask on its own. With the
// amount readable, a currency-only mask still withholds it. No mask leaves the
// bare column.
func TestAFindingsCurrencyAnswersToTheDealCurrencyMask(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		masks  []principal.FieldMask
		masked bool
	}{
		"currency masked": {masks: []principal.FieldMask{{Object: "deal", Field: "currency", Condition: principal.MaskAlways}}, masked: true},
		"no mask":         {masks: nil, masked: false},
	} {
		ctx := principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:reader",
			Permissions: principal.Permissions{
				Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
				FieldMasks: tc.masks,
			},
		})
		arg := func(any) int { return 1 }
		money, err := maskedFindingMoney(ctx, rawFindingMoney.affected, arg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := money.currency != rawFindingMoney.currency; got != tc.masked {
			t.Errorf("%s: currency = %q, want masked=%v", name, money.currency, tc.masked)
		}
		if money.affected != rawFindingMoney.affected {
			t.Errorf("%s: affected = %q, want the bare column — the amount is not masked", name, money.affected)
		}
	}
}

// A read with no principal is refused rather than rendered raw: a slot the
// mask cannot be asked about is never printed as it stands.
func TestAFindingsMoneyIsNotRenderedWithoutAPrincipal(t *testing.T) {
	t.Parallel()
	arg := func(any) int { return 1 }

	money, err := maskedFindingMoney(context.Background(), "masked", arg)
	if err == nil {
		t.Fatalf("maskedFindingMoney with no principal = %+v, want an error", money)
	}
	if money != (findingMoney{}) {
		t.Errorf("maskedFindingMoney returned %+v beside its error, want nothing", money)
	}
}
