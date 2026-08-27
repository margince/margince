// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which of a deal's fields a restore can set back to NOTHING.
//
// Its own file because "which fields this store can clear" is a question a
// reader asks whole, and answering it means reading one place rather than
// hunting through the writer. Applying a clear is storekit's.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// clearableDealColumns names the wire fields a deal restore may set to NULL,
// with literal column names. amount_minor and currency are absent: money is
// read as one field, and a half-cleared pair states an amount in no currency.
// status and the close-date flags belong to the advance path.
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns
func clearableDealColumns(current crmcontracts.Deal) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"expected_close_date": {"expected_close_date", current.ExpectedCloseDate},
		"forecast_category":   {"forecast_category", current.ForecastCategory},
		"wait_until":          {"wait_until", current.WaitUntil},
		"owner_id":            {"owner_id", current.OwnerId},
		"project_id":          {"project_id", current.ProjectId},
	}
}
