// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/customfields"
)

func newAutomationHandlers(pool *pgxpool.Pool) automation.Handlers {
	db := InstallationDB(pool)
	return automation.NewHandlers(db).
		WithQuietPopulationScan(newQuietPopulationReader(db)).
		WithStageLookup(NewProviderFor(db)).
		WithInquiryRouting(newInquiryRouting(db)).
		WithFieldCatalog(customfields.NewService(pool, nil)).
		WithRetryEngine(NewWorkflowEngine(db))
}
