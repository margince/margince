// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package duequestion

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Register puts the question's accept effect and edit check on the deciding
// approvals service.
func Register(svc *approvals.Service, db *database.DB, tasks *activities.Store, now func() time.Time) {
	svc.WithEffect(Kind, Effect(svc, tasks))
	svc.WithPrecheck(Kind, Precheck(db, now))
}
