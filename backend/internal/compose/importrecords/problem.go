// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

import (
	"strings"

	"github.com/margince/margince/backend/internal/compose/importcells"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// CellProblem is one cell the store would refuse. Target is the mapping target;
// the part writer turns it into the file's own header.
type CellProblem struct {
	Target, Value, Message string
	// Why is Message as a code, so the report can say it in another language.
	Why migration.Reason
}

// ProblemOf is one cell the store would refuse, worded by the reason.
func ProblemOf(target, value string, why migration.Reason) *CellProblem {
	return &CellProblem{Target: target, Value: value, Message: why.English(), Why: why}
}

// CheckMoney reads an amount cell in the minor units of its currency, which must
// be named beside it, and puts both in typed. It answers the cell the stores
// would refuse, or nil.
func CheckMoney(fields map[string]string, typed map[string]any, amountTarget, currencyTarget string) *CellProblem {
	amount := strings.TrimSpace(fields[amountTarget])
	currency := strings.ToUpper(strings.TrimSpace(fields[currencyTarget]))
	if currency != "" {
		if !values.AssignableCurrency(currency) {
			return ProblemOf(currencyTarget, fields[currencyTarget],
				migration.Why(migration.ReasonCurrencyNotACode, "value", currency))
		}
		// Only the import refuses these: in a file they are slips for a real code.
		if !values.NamesMoney(currency) {
			return ProblemOf(currencyTarget, fields[currencyTarget],
				migration.Why(migration.ReasonCurrencyNotMoney, "value", currency))
		}
		typed[currencyTarget] = currency
	}
	if amount == "" {
		return nil
	}
	if currency == "" {
		return ProblemOf(amountTarget, fields[amountTarget],
			migration.Why(migration.ReasonAmountNeedsCurrency, "value", amount))
	}
	minor, ok := values.MinorUnits(amount, currency)
	if !ok {
		return ProblemOf(amountTarget, fields[amountTarget], importcells.AmountReason(amount, currency))
	}
	typed[amountTarget] = minor
	return nil
}
