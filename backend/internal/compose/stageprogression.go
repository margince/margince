// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning a stage decision into a card, and a decided card into a moved deal.
//
// The seam lives here because it joins two modules: deals owns the policy and
// the deal, approvals owns the staging and the redemption, and neither may
// import the other. What the adapter adds is the ORDER — decide, stage,
// record — and the guarantee that the approval is spent if and only if the
// deal actually moved.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageProgressionActor is what the audit trail names as the proposer. No
// human asked for the card: evidence landing is what produced it.
const stageProgressionActor = "system:stage-progression"

// StageProgressionProposer decides one deal's stage and stages the card.
type StageProgressionProposer struct {
	pool     *pgxpool.Pool
	deals    *deals.Store
	approval *approvals.Service
	// workspace answers the installation's own workspace. RESOLVED rather than
	// read off the context: both callers are bus-driven, an envelope carries no
	// tenant, and WithWorkspaceTx below reads the context — so without this
	// every proposal from the deterministic lane opens a transaction bound to
	// the zero workspace and is refused.
	workspace func(context.Context) (ids.WorkspaceID, error)
	now       func() time.Time
	log       *slog.Logger
}

// NewStageProgressionProposer builds the proposer over the SAME approvals
// service the HTTP surface decides on, so a released effect can redeem the
// proposal this staged.
func NewStageProgressionProposer(
	pool *pgxpool.Pool, store *deals.Store, approval *approvals.Service,
	now func() time.Time, log *slog.Logger,
) *StageProgressionProposer {
	return &StageProgressionProposer{
		pool: pool, deals: store, approval: approval,
		workspace: identity.NewService(pool).InstallationWorkspace,
		now:       now, log: log,
	}
}

// StageProgressionProposals builds the proposer the evidence lanes call.
//
// The approvals service is approvalsServiceWithEffects — the SAME registration
// list the HTTP surface decides on. A plainer one would stage cards that no
// effect could redeem: the card would appear, a rep would approve it, and the
// deal would not move.
func StageProgressionProposals(
	pool *pgxpool.Pool, now func() time.Time, log *slog.Logger,
) *StageProgressionProposer {
	return NewStageProgressionProposer(pool,
		deals.NewStore(InstallationDB(pool), DealsInstallation()),
		approvalsServiceWithEffects(pool), now, log)
}

// StageProgressionDecisions is the approvals service a stage card is decided
// on: the SAME registration list the HTTP surface uses, so a decision here runs
// the same effect a rep's click runs.
//
// Exported for the integration suite, which must decide through the real
// registry rather than a service it registers itself — a test that supplies its
// own effect proves the effect it wrote, not the one that ships.
func StageProgressionDecisions(pool *pgxpool.Pool) *approvals.Service {
	return approvalsServiceWithEffects(pool)
}

// Propose reads one deal, decides, and stages a card when the decision says to.
//
// Answers whether a card was staged. FALSE IS THE COMMON ANSWER and not a
// failure: most deals at most moments have an unmet criterion, and a proposer
// that put a card up for each of them would be a proposer nobody reads.
func (p *StageProgressionProposer) Propose(ctx context.Context, dealID ids.DealID) (bool, error) {
	actorCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   stageProgressionActor,
	})
	actorCtx = principal.WithCorrelationID(actorCtx, ids.NewV7())
	// Bound before the first transaction opens. A caller that already carries
	// one — the HTTP surface, a test — resolves the same installation, so this
	// is not a narrowing.
	ws, err := p.workspace(actorCtx)
	if err != nil {
		return false, err
	}
	actorCtx = principal.WithWorkspaceID(actorCtx, ws.UUID)

	// READ AND STAGE IN ONE TRANSACTION. Split in two, the deal can move
	// between them: a rep advances it by hand while the facts are in flight,
	// and the card then describes "Discovery → Negotiation" for a deal already
	// in Negotiation. The version pin staging takes would be the NEW version,
	// so the stale payload would pass validation at redemption and move the
	// deal from wherever it now stands. One transaction, and the pin binds the
	// row the payload was written from.
	staged := false
	if err := database.WithWorkspaceTx(actorCtx, p.pool, func(tx pgx.Tx) error {
		// The autopilot is OFF for now: the per-transition policy and the
		// measured rates that would turn it on are their own package, and a
		// proposer that guessed at them would auto-apply on an installation
		// that had measured nothing. Everything reaches a human until then.
		facts, err := deals.ReadStageProgressionFacts(
			actorCtx, tx, dealID, deals.AutopilotFacts{}, p.now())
		if err != nil {
			return err
		}
		if facts.Decision.Outcome == deals.OutcomeObserve {
			if facts.Decision.Exception != "" {
				// An exception is a fact a rep needs told rather than merely
				// acted on. It is logged here and surfaced on the deal's own
				// attention lane rather than staged as a card: there is
				// nothing to decide.
				p.log.InfoContext(actorCtx, "stage progression: an exception on the evidence",
					"deal", dealID.String(), "exception", facts.Decision.Exception)
			}
			return nil
		}
		staged = true
		return p.stageCard(actorCtx, tx, facts)
	}); err != nil {
		if errors.Is(err, deals.ErrNoNextStage) || errors.Is(err, apperrors.ErrNotFound) {
			// A deal on its last open stage, or one archived since the
			// evidence landed. Neither is a failure.
			return false, nil
		}
		return false, err
	}
	return staged, nil
}

// stageCard writes the card and its ledger row in ONE transaction, so a proposal
// the inbox shows is a proposal the report counts.
//
// NOT named `stage`, and the name is load-bearing.
// TestEveryBatchStagerPreLocksItsGroup finds batch stagers by name through the
// syntax tree — anything calling StageOrJoinPendingInTx, then anything looping
// over that — and it compares bare identifiers, so a local variable named
// `stage` reads as a call to this method. vcardingest.go has exactly such a
// variable, and this method called `stage` made that gate report a deadlock
// risk in a function that has none. Matching loosely is the gate's own choice
// and the right one: it must never fail SHORT of a real batch stager, and
// precision here would buy tidiness at the cost of a census that can miss its
// subject. So the ambiguity is resolved on this side, by not taking a name a
// caller is likely to bind.
func (p *StageProgressionProposer) stageCard(
	ctx context.Context, tx pgx.Tx, facts deals.StageProgressionFacts,
) error {
	raw, err := json.Marshal(progressionChange(facts))
	if err != nil {
		return fmt.Errorf("render the proposed stage move: %w", err)
	}
	// Canonicalized and hashed, like every other stager. The hash is what binds
	// the card to its payload: RedeemAndApply refuses a redemption whose change
	// differs from the approved one, and an unset hash makes that check compare
	// empty to empty and pass for anything. It is also what tells one proposal
	// from another — without it the plain join matches ANY pending card on the
	// deal, so a reading that learned something new would silently join the
	// stale card instead of superseding it, and the rep would approve a move
	// citing evidence the product had already moved past.
	change, diffHash, err := diffhash.Canonical(raw)
	if err != nil {
		return fmt.Errorf("canonicalize the proposed stage move: %w", err)
	}
	identity, err := json.Marshal(deals.StageProgressionIdentity{ToStageID: facts.ToStageID})
	if err != nil {
		return fmt.Errorf("render the proposal's identity: %w", err)
	}
	approvalID, err := p.approval.StageOrJoinPendingInTx(ctx, tx, approvals.StageInput{
		Kind:           deals.StageProgressionKind,
		ProposedChange: change,
		DiffHash:       diffHash,
		TargetType:     approvalTargetDeal,
		TargetID:       facts.DealID.UUID,
		// No TargetVersion: staging takes the pin ITSELF, in this same
		// transaction (approvals/stagingpins.go), and never from what a caller
		// supplied. The deal is a versioned target, so the card is pinned to
		// the row the payload above was written from — which is only true
		// because the facts read and this staging share one transaction.
		Summary: facts.Decision.Reason,
		// JoinPending with an identity of the TARGET STAGE: a second
		// reading proposing the same move supersedes the first rather than
		// competing with it in the inbox, where approving the stale one
		// would move the deal on a checklist nobody is looking at.
		JoinPending: true,
		Identity:    identity,
		Evidence:    progressionEvidence(facts),
	})
	if err != nil {
		return err
	}
	return p.deals.RecordProgressionProposed(ctx, tx, deals.ProgressionOutcomeInput{
		ApprovalID:    approvalID.UUID,
		DealID:        facts.DealID,
		PipelineID:    facts.PipelineID,
		FromStageID:   facts.FromStageID,
		ToStageID:     facts.ToStageID,
		EvidenceKinds: progressionEvidenceKinds(facts),
		Outcome:       deals.ProgressionProposed,
	})
}

// progressionChange is the payload a human reads.
func progressionChange(facts deals.StageProgressionFacts) deals.StageProgressionChange {
	criteria := make([]deals.ProposedCriterion, 0, len(facts.Criteria))
	for _, c := range facts.Criteria {
		criteria = append(criteria, deals.ProposedCriterion{
			CriterionID: c.CriterionID,
			Key:         c.Key,
			Label:       c.Label,
			Required:    c.Required,
			Met:         c.Met,
			EvidenceIDs: c.EvidenceIDs,
		})
	}
	change := deals.StageProgressionChange{
		DealID:       facts.DealID,
		FromStageID:  facts.FromStageID,
		ToStageID:    facts.ToStageID,
		FromName:     facts.FromName,
		ToName:       facts.ToName,
		Criteria:     criteria,
		Because:      facts.Decision.Reason,
		ConfirmFirst: facts.Decision.Outcome == deals.OutcomeProposeConfirmFirst,
	}
	if facts.NeedsWinReason {
		// Seeded EMPTY so the card draws the control, and empty rather than
		// guessed so the rep answers it themselves. An empty string does not
		// satisfy the bound — the advance refuses it exactly as it refuses an
		// absent one — so a rep who approves without choosing is told what is
		// missing rather than moving a deal on a reason nobody gave.
		empty := ""
		change.WonWithoutContractReason = &empty
		change.WonWithoutContractDetail = &empty
	}
	return change
}

// progressionEvidence is the material each ticked criterion rests on, so the
// human confirming the move can check it rather than trusting the tick.
//
// It cites the RECORD each claim was read from — the contract, the message —
// with that claim's own quoted fragment. Not the ledger row's id under a
// "deal" source type, which is what this did first: that pointer resolves to
// nothing, since the id belongs to deal_stage_evidence and no reader can open
// a deal by it. A citation a reviewer cannot follow is worse than none, because
// it looks checkable.
//
// A source type outside the approvals vocabulary is dropped rather than
// staged. Staging refuses the whole card on an unknown one, and losing a
// citation is a smaller failure than losing the proposal.
func progressionEvidence(facts deals.StageProgressionFacts) []approvals.Evidence {
	var out []approvals.Evidence
	for _, c := range facts.Criteria {
		if !c.Met {
			continue
		}
		for _, src := range c.Sources {
			if !approvals.EvidenceSourceTypeKnown(src.SourceType) {
				continue
			}
			// The criterion's label stands in for a claim recorded with no
			// quoted fragment — a contract turning active settles its
			// criterion by existing, and has no sentence to quote.
			snippet := src.Snippet
			if snippet == "" {
				snippet = c.Label
			}
			out = append(out, approvals.Evidence{
				Snippet:    snippet,
				SourceType: src.SourceType,
				SourceID:   src.SourceID,
			})
		}
	}
	return out
}

// progressionEvidenceKinds names the criterion kinds this move rested on, so
// the report can answer which evidence an installation actually accepts rather
// than only how often it says yes.
func progressionEvidenceKinds(facts deals.StageProgressionFacts) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range facts.Criteria {
		if !c.Met || seen[string(c.Kind)] {
			continue
		}
		seen[string(c.Kind)] = true
		out = append(out, string(c.Kind))
	}
	return out
}

// stageProgressionPrecheck refuses a decision the effect could not carry out.
//
// The paperless-win case: a move onto a won stage with no signed agreement
// stages with an EMPTY reason, because only a contact can say why there is no
// paper. Without this check the ordinary Accept button commits the approval,
// the effect then refuses the empty reason, and the card is left approved,
// unredeemable and undecidable while the deal has not moved. A precheck runs
// before the decision commits, so the rep is told what is missing and the card
// stays answerable.
func stageProgressionPrecheck() approvals.ReleasePrecheck {
	return func(_ context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		change, err := deals.ReadProgressionChange(payload)
		if err != nil {
			return &approvals.InvalidEditError{Cause: err}
		}
		if change.WonWithoutContractReason == nil {
			// Not a paperless win. Every ordinary move lands here.
			return nil
		}
		if err := deals.ValidateWonReason(
			*change.WonWithoutContractReason, change.WonWithoutContractDetail); err != nil {
			return &approvals.InvalidEditError{Cause: err}
		}
		return nil
	}
}

// refuseAStaleAutomaticMove holds an automatic apply to the rule as it stands
// NOW, in the transaction that would move the deal.
//
// Answers nil for a human decision without asking anything: the question is
// whether the PRODUCT may still move this by itself, and a contact who pressed
// approve has already answered a different one.
func refuseAStaleAutomaticMove(
	ctx context.Context, tx pgx.Tx, change deals.StageProgressionChange,
) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalAgent || actor.ID != autoApplyActorID {
		return nil
	}
	var pipelineID ids.PipelineID
	if err := tx.QueryRow(ctx,
		`SELECT pipeline_id FROM deal WHERE id = $1`, change.DealID).Scan(&pipelineID); err != nil {
		return fmt.Errorf("compose: read the deal's pipeline for an automatic move: %w", err)
	}
	verdict, err := deals.StageAutopilotModeTx(ctx, tx, deals.TransitionRef{
		PipelineID:  pipelineID,
		FromStageID: change.FromStageID,
		ToStageID:   change.ToStageID,
	}, time.Now())
	if err != nil {
		return err
	}
	if verdict.Mode != deals.ModeAuto {
		// ErrVersionSkew rather than a bare error: the sweep classifies it as
		// a refusal of THIS row and carries on, which is right — the rule
		// changed under a decision that had not landed yet, and every other
		// card still deserves its pass.
		return fmt.Errorf("the rule changed before this move landed (%s): %w",
			verdict.Why, apperrors.ErrVersionSkew)
	}
	return nil
}

// stageProgressionEffect performs an approved move: redeem and advance in ONE
// transaction, so the approval is spent if and only if the deal moved.
//
// The two-transaction shape has a hole with no way out: if the advance fails —
// the deal was archived, the target stage retired — the approval is already
// consumed, cannot be decided again, and nothing else drives the effect. The
// move is lost and the rep is told only that the effect failed.
//
// The move runs under the DECIDING HUMAN's authority, not the system's. A
// contact approving a card is making that move themselves, and the audit trail
// should say so: a stage change attributed to the system would leave nobody
// answerable for a deal that moved.
func stageProgressionEffect(svc *approvals.Service, store *deals.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		change, err := deals.ReadProgressionChange(proposedChange)
		if err != nil {
			return err
		}
		if _, ok := principal.Actor(ctx); !ok {
			return errors.New("compose: a stage progression effect without a deciding principal")
		}
		// Resolved BEFORE the redemption's transaction opens: the catalog
		// reads through its own connection, and a second connection inside
		// somebody else's transaction can deadlock against a lock it holds.
		active, err := store.ActiveDealColumns(ctx)
		if err != nil {
			return err
		}
		return svc.RedeemAndApply(ctx, approvalID, deals.StageProgressionKind, diffHash,
			func(tx pgx.Tx) error {
				// An AUTOMATIC apply re-asks the governing rule here, inside
				// the transaction that moves the deal.
				//
				// The sweep asked before deciding, and that answer is already
				// stale by the time this runs: an admin can flip the kill
				// switch, set the transition back to propose, or the product
				// can suspend the rule in between. Read there and written
				// here, the move commits on a permission that no longer
				// exists — which is exactly what StageAutopilotModeTx's own
				// comment says the transaction is for.
				//
				// A HUMAN's approval skips this. A contact deciding is the
				// authority, and re-asking the autopilot's thresholds would
				// let a suspended rule block a move somebody explicitly made.
				if err := refuseAStaleAutomaticMove(ctx, tx, change); err != nil {
					return err
				}
				_, err := store.AdvanceDealTx(ctx, tx, change.DealID, deals.AdvanceDealInput{
					ToStageID: change.ToStageID,
					// The card this move came from. Without it readProtection
					// reads an accepted proposal as a rep overruling the
					// product, and one approved card silences the next
					// fortnight of them on that deal.
					ApprovalID: &approvalID.UUID,
					// Carried from the card the human decided, not composed
					// here: a win with no agreement behind it is refused
					// unless somebody says why, and the somebody is the contact
					// who approved the move.
					WonWithoutContractReason: change.WonWithoutContractReason,
					WonWithoutContractDetail: change.WonWithoutContractDetail,
				}, active)
				return err
			})
	}
}
