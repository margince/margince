// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The site_lead ACCEPT executor (R5): a human approval of one staged
// published contact captures them as a LEAD through the one capture Sink —
// never directly a contact (ADR-0008: leads graduate; the Sink's own
// cross-source email dedupe stages the 🟡 merge when the contact later
// emails in, and that staged merge is the promotion trigger, not this
// effect). Redeem-then-execute like every 🟡 executor: the single-use
// redemption is the exactly-once claim, and the Sink's natural key makes
// a re-read's re-accept land on the same lead row.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// siteLeadCapturedBy is the executor identity the captured lead carries. The
// Sink requires captured_by to equal the acting principal's id, so one
// spelling covers both.
//
// `connector:` and not `agent:`. The Sink admits a PrincipalConnector alone,
// and storekit stamps actor_type from the type beside actor_id from this. Both
// prefixes read as machine-captured where provenance is matched by prefix, so
// this spelling moves no row between those arms.
const siteLeadCapturedBy = "connector:siteread"

// siteLeadAcceptEffect builds the approvals.ApprovedEffect compose
// injects for kind "site_lead".
func siteLeadAcceptEffect(
	svc *approvals.Service, sink connector.Sink, pool *pgxpool.Pool, notices *siteLeadNoticeLane,
) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		// siteLeadPrecheck refuses a single or bundled approval before it
		// commits. This is the check for every path that reaches the effect
		// without one, a re-driven decision included.
		if err := notices.refusal(); err != nil {
			return err
		}
		var proposal siteLeadProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the site_lead proposal: %w", err)
		}
		// A proposal staged before the payload carried its natural key still
		// has to land on the right lead. An empty key would put every such
		// contact on one row, so derive it from what the payload carries.
		if proposal.NaturalKey == "" {
			proposal.NaturalKey = siteLeadSourceID(
				proposal.CompanyID, proposal.Name, proposal.PublishedEmail)
		}
		// The redemption is the idempotency claim on the APPROVAL and the
		// Sink's natural key the claim on the LEAD. Both commit with the
		// notice, or with the record of why it was not given.
		return sendArticle14Notice(ctx, pool, notices, approvalID, proposal, siteLeadSteps{
			redeem: func(ctx context.Context) error {
				_, _, err := svc.Redeem(ctx, approvalID, siteLeadProposalKind, diffHash)
				return err
			},
			capture: func(ctx context.Context) (ids.LeadID, bool, error) {
				return captureSiteLead(ctx, pool, sink, proposal, proposedChange)
			},
		})
	}
}

// captureSiteLead captures the accepted contact through the one capture Sink,
// and answers whether the lead it landed on is this proposal's own.
func captureSiteLead(
	ctx context.Context, pool *pgxpool.Pool, sink connector.Sink, proposal siteLeadProposal, raw json.RawMessage,
) (ids.LeadID, bool, error) {
	// The capture runs as the siteread executor for the human whose approval
	// released it. The Sink admits connector principals only, so the executor
	// is one. It carries the decider's own live RBAC and row scope, as the
	// capture registry's connectors carry their granting human's. So the
	// effect can capture nothing the deciding human could not.
	decider, ok := principal.Actor(ctx)
	if !ok {
		return ids.LeadID{}, false, errors.New("compose: site_lead effect without a deciding principal")
	}
	execCtx := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalConnector,
		ID:          siteLeadCapturedBy,
		UserID:      decider.UserID,
		OnBehalfOf:  decider.UserID,
		TeamIDs:     decider.TeamIDs,
		SeatType:    decider.SeatType,
		Permissions: decider.Permissions,
	})
	// The staged proposal itself is the raw original: it carries the role,
	// the evidence snippet, the source URL, and the company the read targeted.
	ref, err := sink.Upsert(execCtx, connector.NormalizedRecord{
		EntityType: datasource.EntityLead,
		NaturalKey: connector.NaturalKey{
			SourceSystem: siteLeadSourceSystem,
			// The key the proposal was STAGED under, so the row a human
			// accepts is the row the inbox deduplicated on. Recomputing it
			// here would agree only by both sides calling the same
			// function, and would drift the moment one of them changed.
			SourceID: proposal.NaturalKey,
		},
		Fields: capture.LeadFields{
			FullName: proposal.Name,
			Email:    proposal.PublishedEmail,
			Title:    proposal.Role,
			// Accepting a name read off a website records that the contact
			// exists, not that the accepter has taken them on. Owning the lead
			// would start a first-response clock for somebody who never wrote.
			// Nobody owns it until a human picks it up in Leads.
			Unowned: true,
		},
		Source:     siteLeadSourceSystem,
		CapturedBy: siteLeadCapturedBy,
		Raw:        raw,
	})
	if err != nil {
		return ids.LeadID{}, false, err
	}
	// The sink keeps no lead for an existing contact. The identity locks decide
	// this independently of what the requester could read when it was staged.
	if ref.Type == datasource.EntityContact {
		return ids.LeadID{}, false, errSiteLeadIsAContact
	}
	leadID := ids.From[ids.LeadKind](ref.ID)
	// A collision with another source's lead returns that lead, and stages a
	// merge for a human instead of writing this one.
	var ours bool
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM lead WHERE id = $1 AND source_system = $2 AND source_id = $3)`,
			leadID, siteLeadSourceSystem, proposal.NaturalKey).Scan(&ours)
	})
	return leadID, ours, err
}

// siteLeadSourceSystem is the source a website lead is captured under, which
// consent asks before it sends the notice.
const siteLeadSourceSystem = provenance.WebsiteLeadSource
