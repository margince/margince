// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:stage-progression-outcome consumer: a decided card becomes a counted
// one.
//
// It lives here because the question crosses two modules: approvals owns the
// verdict, deals owns the ledger the launch gate reads, and neither imports the
// other.
//
// A CONSUMER rather than a hook on the decision path, for a reason the expiry
// sweep already relies on (jobs_approvalexpiry.go says it at length): the
// window closing on a card nobody answered is a REAL outcome, written by the
// sweep and not by any human, and it reaches the same approval.decided event
// with verdict `expired`. A hook on the human decide path would count the
// answered cards and silently miss those — leaving every unanswered transition
// permanently at `proposed`, which reads as a clean record rather than as an
// unmeasured one. One consumer closes both, and the delivery guarantee is the
// outbox's rather than something this file reproduces.
//
// Idempotency is the ledger row's own status: RecordProgressionDecided matches
// only a row still standing at `proposed`, so a redelivered verdict finds
// nothing to do and the FIRST decision — the one that happened — stands.
// events.Dedupe sits in front of that as a cache, never as the guarantee.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageProgressionOutcomeActor names this consumer in the audit rows it causes,
// so a card the clock closed is told apart from one a person answered.
const stageProgressionOutcomeActor = "system:stage-progression-outcome"

// StageProgressionOutcome records how each staged move was received.
type StageProgressionOutcome struct {
	pool     *pgxpool.Pool
	deals    *deals.Store
	identity *identity.Service
	log      *slog.Logger
}

// NewStageProgressionOutcome builds the consumer over the deal store that owns
// the ledger.
func NewStageProgressionOutcome(
	pool *pgxpool.Pool, store *deals.Store, ident *identity.Service, log *slog.Logger,
) *StageProgressionOutcome {
	return &StageProgressionOutcome{pool: pool, deals: store, identity: ident, log: log}
}

// HandleEvent routes one envelope. Anything that is not a decision on a stage
// progression answers nil, so the consumer group keeps flowing.
func (o *StageProgressionOutcome) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != string(crmcontracts.ApprovalDecided) || env.Entity.ID == ids.Nil {
		return nil
	}
	var payload struct {
		Kind    string `json:"kind"`
		Verdict string `json:"verdict"`
		Edited  bool   `json:"edited"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return fmt.Errorf("stage progression outcome: approval.decided payload: %w", err)
	}
	// The kind filter comes from the payload rather than from a ledger lookup,
	// so the overwhelming majority of decisions — every other approval kind in
	// the product — cost no query at all.
	if payload.Kind != deals.StageProgressionKind {
		return nil
	}
	outcome, ok := progressionOutcomeFor(payload.Verdict, payload.Edited)
	if !ok {
		// A verdict this consumer does not know is not a silent skip: the
		// ledger is what the launch gate reads, so an unrecognised one would be
		// measured as a card still awaiting an answer.
		return fmt.Errorf(
			"stage progression outcome: %q is not a verdict this ledger records", payload.Verdict)
	}

	// The envelope carries no tenant, and the subscriber binds none: without
	// this the workspace resolves to zero and every write is refused.
	ws, err := o.identity.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.EventID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: stageProgressionOutcomeActor,
	})
	// evidence_corrected is FALSE, not payload.Edited, and the two are not the
	// same question. That column records a human marking a CLAIM incorrect;
	// `edited` records that the approver changed the payload — and the only
	// editable field on this card is the paperless-win reason. Filling in a
	// required reason would otherwise write "the evidence was corrected", which
	// is the one dimension the migration keeps deliberately apart from the
	// outcome. A real correction is a refutation on deal_stage_evidence, and
	// the writer that records one is where this column will be set.
	//
	// The rejection's REASON is not carried either: the payload does not have
	// it, and inventing one is worse than the approval row keeping it.
	return o.deals.RecordProgressionDecided(ctx, env.Entity.ID, outcome, nil, false)
}

// progressionOutcomeFor maps a verdict onto what the ledger counts.
//
// `expired` is a refusal with no decider, and it is counted as its own outcome
// rather than folded into a rejection: a card nobody looked at says the product
// asked at a moment nobody was reading, which is a different failure from a rep
// looking at the evidence and saying no.
//
// ProgressionAutoApplied is NOT reachable from here, and deliberately so. An
// automatic apply is marked by the approval row's decided_by_system column,
// which approval.decided does not carry — so this consumer cannot tell one from
// a human's clean approval, and a branch guessing at it would file the
// autopilot's own moves under the rate that measures whether humans agree with
// it. Nothing produces one yet either: the proposer reads an empty
// AutopilotFacts, so every card in this release reaches a person. The autopilot
// PR owns both halves — widening the event, and this arm.
func progressionOutcomeFor(verdict string, edited bool) (string, bool) {
	switch crmcontracts.PublicEventApprovalDecidedVerdict(verdict) {
	case crmcontracts.Approved:
		if edited {
			return deals.ProgressionApprovedEdited, true
		}
		return deals.ProgressionApprovedClean, true
	case crmcontracts.Rejected:
		return deals.ProgressionRejected, true
	case crmcontracts.Expired:
		return deals.ProgressionExpired, true
	}
	return "", false
}
