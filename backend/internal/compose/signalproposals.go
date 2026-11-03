// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning an observation into an offer (the account-intelligence proposals
// arc).
//
// A signal says what the correspondence stated. It changes nothing. This is
// where the consequence is proposed: an account whose own mail ends the
// contract is offered the move to former_customer. The reconciler only stages
// it. A human decision or the owner's automatic-change policy releases the
// effect; the applier resolves the owner's current permissions before writing.
//
// The model never stages. This whole file is deterministic: it reads open
// signals and open approvals, and it proposes exactly one thing per open
// contract_ended signal. With automatic changes off, the proposal waits for
// manual review before its effect changes the account.
//
// The precedent is company_name_promotion, not deal_follow_up: StageUnlessDeclined
// (durable rejection memory — StageOrJoinPendingInTx only dedupes against live
// pending rows and forgets rejections) and RedeemAndApply (redemption and the
// write in one transaction).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	// lifecycleProposalKind names the staged offer in the review queue.
	lifecycleProposalKind = "lifecycle_change"
	// lifecycleProposalActor is the provenance the accepted write carries: the
	// stage came from the account's own correspondence, not from someone
	// typing it, and a later human edit must still win over it.
	lifecycleProposalActor = "agent:" + lifecycleProposalKind
	// signalKindContractEnded is the observation this reconciler acts on,
	// spelled once: the read that finds contradictions and the effect that
	// settles them must agree, and two literals would drift.
	signalKindContractEnded = "contract_ended"
	// lifecycleEnded is what a contract_ended signal proposes from every live
	// stage. The founder's own example is a record reading "Prospect" whose
	// mail ends a contract: the mail is the fact, and former_customer is what
	// it says, whether or not the record ever said customer.
	lifecycleEnded = "former_customer"
)

// lifecycleProposal is the staged change: which account, from what stage to
// what stage, and the signal that asked for it.
//
// The current stage is part of the payload because the card must show both
// sides — a reader deciding "is this right?" needs to see what the record says
// now, not only what it would say next.
type lifecycleProposal struct {
	Evidence      *lifecyclereview.Finding `json:"evidence,omitempty"`
	CompanyID     ids.CompanyID            `json:"company_id"`
	CurrentStage  string                   `json:"current_lifecycle"`
	ProposedStage string                   `json:"proposed_lifecycle"`
	SignalID      ids.UUID                 `json:"signal_id"`
	// Because is the signal's own summary, so the card can say why in the
	// words the conversation used rather than in a rule's words.
	Because string `json:"because"`
}

// lifecycleIdentity is what a refusal is REMEMBERED by: the account and the
// stage it was offered. Not the payload — the payload carries the signal's
// summary and the record's current stage, both of which move on their own, and
// a refusal keyed on the whole payload would be forgotten the first time
// either did, re-offering the same move on every pass until someone accepted.
func lifecycleIdentity(companyID ids.CompanyID, stage string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]string{
		paramCompanyID: companyID.String(), "proposed_lifecycle": stage,
	})
	if err != nil {
		return nil, fmt.Errorf("compose: encoding the lifecycle proposal identity: %w", err)
	}
	return body, nil
}

// SignalProposer offers the consequence of a signal to a human.
type SignalProposer struct {
	now       func() time.Time
	pool      *pgxpool.Pool
	approvals *approvals.Service
	log       *slog.Logger
}

// NewSignalProposer builds the reconciler over the pool and one approvals
// service — the same instance that decides, so a released effect can redeem
// what it decides on.
func NewSignalProposer(pool *pgxpool.Pool, svc *approvals.Service, log *slog.Logger) *SignalProposer {
	return &SignalProposer{pool: pool, approvals: svc, log: log, now: time.Now}
}

// contradiction is one account whose stage its own mail contradicts.
type contradiction struct {
	Finding   lifecyclereview.Finding
	CompanyID ids.CompanyID
	Stage     string
	SignalID  ids.UUID
	Summary   string
}

// RunWorkspace offers a stage change for every account whose open
// contract_ended signal contradicts the stage it is filed under, and reports
// how many of those offers are now STANDING in the inbox.
//
// Standing, not newly created: an hourly pass over an unchanged account joins
// the offer it made last hour rather than stacking a second copy, and both
// count. The number a reader wants from this is "how many accounts are waiting
// on a decision", which is the same on every pass until someone decides.
//
// Staging happens OUTSIDE the signal-write transaction: StageUnlessDeclined
// has no in-tx variant, because it takes the approval row lock itself. A crash
// between the signal and the offer self-heals — the signal is still open on
// the next hourly pass, and this reads open signals rather than new ones.
func (p *SignalProposer) RunWorkspace(ctx context.Context) (int, error) {
	var found []contradiction
	var said approvalSummaryCopy
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		if found, err = readContradictions(ctx, tx, p.now()); err != nil {
			return err
		}
		said = approvalSummaryCopyIn(ctx, tx)
		return nil
	}); err != nil {
		return 0, fmt.Errorf("compose: reading the accounts their mail contradicts: %w", err)
	}
	standing := 0
	for _, account := range found {
		live, err := p.offerStageChange(ctx, said, account)
		if err != nil {
			return standing, err
		}
		if live {
			standing++
		}
	}
	return standing, nil
}

func readContradictions(ctx context.Context, tx pgx.Tx, now time.Time) ([]contradiction, error) {
	// ONE row per account, newest signal first. The question this offers is
	// about the account's stage, and an account whose mail says the contract
	// ended in three conversations is not three questions — staging once per
	// signal made each pass supersede the last, leaving whichever row happened
	// to be first standing and the rest never asked about at all.
	args := []any{values.EngagedLifecycles()}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT DISTINCT ON (o.id) o.id, o.lifecycle, s.id, s.summary
		  FROM signal s
		  JOIN company o ON o.id = s.resolved_company_id AND o.archived_at IS NULL
		 WHERE s.kind = '`+signalKindContractEnded+`' AND s.status = 'open'
		   AND s.archived_at IS NULL AND o.lifecycle = ANY($%d)
		 ORDER BY o.id, s.detected_at DESC, s.id DESC`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contradiction
	for rows.Next() {
		var found contradiction
		if err := rows.Scan(&found.CompanyID, &found.Stage,
			&found.SignalID, &found.Summary); err != nil {
			return nil, err
		}
		out = append(out, found)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	kept := out[:0]
	for _, account := range out {
		if !values.EngagedLifecycle(account.Stage) {
			continue
		}
		finding, err := lifecyclereview.Reconcile(ctx, tx, account.CompanyID, now)
		if refused(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if lifecyclereview.Actionable(finding) {
			account.SignalID, account.Summary = finding.SignalID, finding.Summary
			account.Finding = finding
			kept = append(kept, account)
		}
	}
	return kept, nil
}

// offerStageChange puts one offer in the inbox, reporting whether an offer now
// stands there. A false means a human already refused this exact move on this
// account, and the answer is not to ask again.
//
// JoinPending keeps an hourly re-run from stacking the same question in the
// inbox, and StageUnlessDeclined keeps a human's "no" from being asked again
// next hour — the signal that produced it stays open, so without the durable
// memory this offer would come back every pass forever.
func (p *SignalProposer) offerStageChange(ctx context.Context, said approvalSummaryCopy, account contradiction) (bool, error) {
	identity, err := lifecycleIdentity(account.CompanyID, lifecycleEnded)
	if err != nil {
		return false, err
	}
	body, err := json.Marshal(lifecycleProposal{
		Evidence:      &account.Finding,
		CompanyID:     account.CompanyID,
		CurrentStage:  account.Stage,
		ProposedStage: lifecycleEnded,
		SignalID:      account.SignalID,
		Because:       approvals.SanitizeSummary(account.Summary),
	})
	if err != nil {
		return false, fmt.Errorf("compose: encoding the lifecycle proposal: %w", err)
	}
	digest := sha256.Sum256(body)
	_, live, err := p.approvals.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           lifecycleProposalKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		Identity:       identity,
		TargetType:     string(recordTypeCompany),
		TargetID:       account.CompanyID.UUID,
		Summary:        fmt.Sprintf(said.contractEndedMove, account.Stage, lifecycleEnded),
		JoinPending:    true,
		Evidence:       []approvals.Evidence{{Snippet: approvals.SanitizeSummary(account.Summary), SourceType: tableSignal, SourceID: account.SignalID}},
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return live, nil
}

// lifecycleAcceptEffect builds the approvals.ApprovedEffect for kind
// "lifecycle_change": the human agreed with what the correspondence said, so
// the stage moves and the signal that asked is acknowledged — in ONE
// transaction, because a moved account still shouting the signal that moved it
// is a page that contradicts itself.
//
// There is no reject effect. Rejecting means the reader judged the stage
// right and the reading wrong; the record is already what they want, and the
// signal stays open because they did not say the mail was wrong, only that
// the record was not.
func lifecycleAcceptEffect(svc *approvals.Service, store *contacts.Store, now func() time.Time) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal lifecycleProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the lifecycle proposal: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: lifecycle accept without a deciding principal")
		}
		// The write carries the machine provenance — the stage came from the
		// account's correspondence, not from someone typing it — while the
		// human's approval is on the decision's own audit row.
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         lifecycleProposalActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		return svc.RedeemAndApply(ctx, approvalID, lifecycleProposalKind, diffHash, func(tx pgx.Tx) error {
			if err := lifecyclereview.Revalidate(ctx, tx, proposal.CompanyID, proposal.SignalID, proposal.Evidence, now()); err != nil {
				return err
			}
			// A false here is the stage having been corrected by a human while
			// the offer waited: the approval is spent, nothing is written, and
			// their edit stands.
			moved, err := store.SetCompanyLifecycleTx(execCtx, tx,
				proposal.CompanyID, proposal.CurrentStage, proposal.ProposedStage)
			if err != nil || !moved {
				return err
			}
			_, err = signals.AcknowledgeCoveredForCompanyTx(execCtx, ctx, tx,
				proposal.CompanyID.UUID, signalKindContractEnded, []ids.UUID{proposal.SignalID})
			return err
		})
	}
}
