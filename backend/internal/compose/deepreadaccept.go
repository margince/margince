// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep-read ACCEPT executor (R4): a human approval of a staged
// "deepread" proposal lands the whole read in one transaction — profile
// fields fill-empty like a quick scrape, category facts into
// company_fact — with human-set values taking precedence on both.
// Redeem-then-execute like every 🟡 executor: the single-use redemption
// is the exactly-once claim, so a replayed or re-driven decision applies
// nothing twice.

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deepReadAcceptEffect builds the approvals.ApprovedEffect compose
// injects for kind "deepread".
func deepReadAcceptEffect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		proposal, err := contacts.UnmarshalDeepRead(proposedChange)
		if err != nil {
			return err
		}
		// The write executes as the deep-read agent acting for the human whose
		// approval released it, under that human's grants. This row carries
		// the machine provenance the 360 renders as "read from the company's site".
		execCtx, err := agentForActor(ctx, "agent:deepread")
		if err != nil {
			return err
		}
		return svc.RedeemAndApply(ctx, approvalID, deepReadProposalKind, diffHash, func(tx pgx.Tx) error {
			return store.ApplyDeepReadTx(execCtx, tx, proposal)
		})
	}
}
