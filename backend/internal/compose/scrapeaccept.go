// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The enrichment ACCEPT executor (EP05): a human approval of a staged
// scrapeCompany proposal WRITES the accepted fields onto the company the proposal
// named — fill-empty-only, evidence queryable. Redeem-then-execute like every
// 🟡 executor: the single-use redemption is the exactly-once claim, so a
// replayed or re-driven decision applies nothing twice.

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scrapeAcceptEffect builds the approvals.ApprovedEffect compose injects for
// kind "enrich".
func scrapeAcceptEffect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		companyID, sourceURL, fields, err := contacts.UnmarshalEnrichment(proposedChange)
		if err != nil {
			return err
		}
		// The write executes as the scrape agent acting for the human whose
		// approval released it, under that human's grants: captured_by =
		// agent:scrape. That human is on the decision's own audit row, and
		// this one carries the machine provenance the 360 renders as "read
		// from the company's site".
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: enrich effect without a deciding principal")
		}
		agent, err := agentPrincipalFor(decider, "agent:scrape")
		if err != nil {
			return err
		}
		execCtx := principal.WithActor(ctx, agent)
		// The single-use redemption IS the idempotency claim: whoever consumes
		// the approval executes; anyone else finds it consumed. Redeemed in the
		// write's OWN transaction, so a failed apply leaves the approval
		// unconsumed and retryable — spent first, a write that then fails loses
		// the change with no path back, because Decide refuses a second
		// decision and nothing else drives this effect.
		// Redeemed under the DECIDER's context and applied under the executor's:
		// the redemption is an assertion of that human's authority, and the
		// write carries the machine provenance the 360 renders.
		return svc.RedeemAndApply(ctx, approvalID, enrichProposalKind, diffHash, func(tx pgx.Tx) error {
			return store.ApplyEnrichmentTx(execCtx, tx, companyID, contacts.ApplyColdStartProfileInput{
				SourceURL: sourceURL,
				Fields:    fields,
			})
		})
	}
}
