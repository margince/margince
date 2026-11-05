// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promotionContextPin binds an approved promotion to the company decision and
// evidence it was reviewed on. It reads either door's payload, as the preview does.
func promotionContextPin(db *database.DB) approvals.ContextPin {
	store := newContactsStore(db)
	return func(ctx context.Context, tx pgx.Tx, leadID ids.UUID, proposedChange json.RawMessage) (string, error) {
		change, err := diffhash.DecodeObject(proposedChange)
		if err != nil {
			return "", err
		}
		target := string(recordTypeLead)
		in, err := promotionRequestInput(crmcontracts.Approval{
			TargetEntityType: &target, TargetEntityId: new(openapi_types.UUID(leadID)), ProposedChange: &change,
		})
		if err != nil {
			return "", err
		}
		return store.PromotionContextDigestTx(ctx, tx, ids.From[ids.LeadKind](leadID), in)
	}
}
