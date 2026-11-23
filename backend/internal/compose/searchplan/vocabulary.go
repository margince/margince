// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"encoding/json"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Vocabulary carries caller-visible reader catalogs without CRM record values.
type Vocabulary struct {
	Now                 time.Time                     `json:"now"`
	Context             crmcontracts.AnalyticsContext `json:"context"`
	Metrics             crmcontracts.ReportingCatalog `json:"metrics"`
	SelfUserID          string                        `json:"self_user_id"`
	BaseCurrencyScale   int64                         `json:"base_currency_scale"`
	AnalyticsVocabulary string                        `json:"analytics_vocabulary"`
	RecordVocabulary    json.RawMessage               `json:"record_vocabulary"`
	MinorUnitExceptions map[string]int                `json:"minor_unit_exceptions"`
}

// MarshalJSON adds the money operand rule the admission applies. A field named
// for the minor units it stores is then not read as asking for them.
func (v Vocabulary) MarshalJSON() ([]byte, error) {
	type wire Vocabulary
	return json.Marshal(struct {
		wire
		MoneyOperands string `json:"money_operands"`
	}{wire(v), moneyOperandRule})
}

// moneyOperandRule states, from the suffixes admission scales, the unit a
// planner writes a money operand in.
const moneyOperandRule = "A filter on a field ending " + moneySuffix + " or " + baseMoneySuffix +
	" takes its operand in MAJOR units (50k is 50000); the server scales it to the stored minor units."
