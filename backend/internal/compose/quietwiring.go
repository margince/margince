// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/quietreminders"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
)

func newQuietPopulationReader(db *database.DB) quietreminders.Reader {
	return quietreminders.NewReader(quietScanStore(db), NewCollectionsStore(db.Pool()), deals.NewStore(db, DealsInstallation()))
}
