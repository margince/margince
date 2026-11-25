// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package proposalstage

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Register puts the accept effect and the precheck on the deciding approvals
// service. A declined question needs no effect: the inbox remembers it per
// source.
func Register(svc *approvals.Service, db *database.DB, store *deals.Store) {
	svc.WithEffect(deals.ProposalStageKind, AcceptEffect(svc, store))
	svc.WithPrecheck(deals.ProposalStageKind, Precheck(db))
}
