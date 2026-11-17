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
