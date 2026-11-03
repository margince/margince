// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/mailcoverage"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
)

func mailCaptureCoverageReader(db *database.DB) mailcoverage.Reader {
	registry := captureHealthRegistry(db)
	return func(ctx context.Context, from *time.Time, at time.Time) (crmcontracts.MailCaptureCoverage, error) {
		return registry.MailCoverage(ctx, MailProviders(), from, at)
	}
}
