// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company-name promotion sweep (PO-F-2a, ADR-0072 phase 3): the follow-on
// to the signature-enrich pass, and the only consumer of the `company_name`
// evidence that pass collects.
//
// A captured company is named from its mail domain and marked provisional.
// This sweep replaces that name with the one the company's own people sign
// with, but only when a second independent source agrees — the site dossier or
// a second employee. A lone signature is not overruled and not obeyed either:
// it becomes a 🟡 proposal, and a human decides.
//
// No model call, no network: everything it weighs is already in the database,
// so it runs in the same River job as the enrich pass that produced the
// evidence rather than paying for a schedule of its own.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// companyNameProposalKind names the staged offer in the review queue.
	companyNameProposalKind = "company_name_promotion"
	// companyNameTargetType is what the proposal points at — the company
	// whose name is in question, so the target's own visibility decides who
	// may see and decide the offer.
	companyNameTargetType = "company"
	// companyNamePromotionActor is the principal the sweep and the accept write as.
	companyNamePromotionActor = "agent:" + companyNameProposalKind
	// companyNamePromotionPageSize bounds ONE page of candidates — a memory bound,
	// not a work bound: the pass reads every candidate, a page at a time.
	companyNamePromotionPageSize = 200
	// companyNamePromotionMaxPages is a runaway backstop, not a policy. A workspace
	// that reaches it has more provisionally-named companies than any real
	// installation, and the pass says so rather than trimming the work silently.
	companyNamePromotionMaxPages = 500
)

// companyNameProposal is the staged offer's payload: the name being proposed, the
// company it would replace, and the evidence a reviewer judges it on.
//
// It carries the CURRENT name as well, because the offer is a diff: an
// company renamed by someone else while the proposal sat in the inbox is
// no longer the change a human was shown, and the accept's CAS on
// name_source='domain' is what refuses it.
type companyNameProposal struct {
	CompanyID    ids.CompanyID `json:"company_id"`
	CurrentName  string        `json:"current_name"`
	ProposedName string        `json:"proposed_name"`
	// ProposedNameKey is ProposedName normalized. It is carried on the payload
	// because the staging identity is a subset of it: the identity is what a
	// human's refusal is remembered by, and it must survive a change of
	// spelling, of evidence, or of the name the record currently holds.
	ProposedNameKey string `json:"proposed_name_key"`
	// Persons are the people whose signatures state the proposed name.
	Persons []ids.PersonID `json:"persons"`
}

// companyNameIdentity is the logical identity of a company-name proposal: WHICH record
// would be renamed to WHICH name. Everything else in the payload is evidence,
// and evidence moves — a new signer, a corrected spelling, the record's current
// name changing — while the question a human answered stays the same one.
func companyNameIdentity(companyID ids.CompanyID, nameKey string) (json.RawMessage, error) {
	identity, err := json.Marshal(map[string]string{
		paramCompanyID:      companyID.String(),
		"proposed_name_key": nameKey,
	})
	if err != nil {
		return nil, fmt.Errorf("compose: encoding the company-name proposal identity: %w", err)
	}
	return identity, nil
}

// refusedNameKey reports whether any of the refused payloads named THIS claim.
//
// The comparison is on the normalized key, never on the spelling. An offer
// staged before proposed_name_key existed carries only the raw name, and the
// raw name is dominantSpelling's pick — it moves as signatures accumulate, so
// two spellings of one refused claim would otherwise read as two different
// questions and the human's answer to the first would not bind the second.
// Normalizing the stored name is what makes an old refusal mean the same thing
// a new one does.
func refusedNameKey(refused []json.RawMessage, nameKey string) bool {
	for _, raw := range refused {
		var prior companyNameProposal
		if err := json.Unmarshal(raw, &prior); err != nil {
			// A payload this kind cannot read is not this kind's refusal.
			continue
		}
		key := prior.ProposedNameKey
		if key == "" {
			key = people.NormalizeCompanyName(prior.ProposedName)
		}
		if key != "" && key == nameKey {
			return true
		}
	}
	return false
}

// CompanyNamePromoter runs the sweep for every workspace.
type CompanyNamePromoter struct {
	pool      *pgxpool.Pool
	store     *people.Store
	approvals *approvals.Service
	log       *slog.Logger
}

// NewCompanyNamePromoter builds the sweep over the pool. Its approvals service is
// the staging half only: the accept EFFECT is registered on the service the
// HTTP surface decides through (approvalsServiceWithEffects), which is where a
// human's decision arrives.
func NewCompanyNamePromoter(pool *pgxpool.Pool, log *slog.Logger) *CompanyNamePromoter {
	return &CompanyNamePromoter{
		pool:      pool,
		store:     people.NewStore(InstallationDB(pool)),
		approvals: approvals.NewService(InstallationDB(pool)),
		log:       log,
	}
}

// RunWorkspace weighs one workspace's provisionally-named companies against
// their signature evidence. One company's failure is logged and skipped:
// the evidence is durable, so the next pass sees exactly the same question.
//
// The fleet fan-out lives in the job layer, so a workspace whose pass fails
// fails its own job row.
func (p *CompanyNamePromoter) RunWorkspace(ctx context.Context, ws ids.UUID) error {
	// The promotion writes an audit row and a company.updated event,
	// so the pass binds the system actor like every worker job.
	wsCtx := principal.WithCorrelationID(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   companyNamePromotionActor,
	}), ids.NewV7())
	return p.sweepWorkspace(wsCtx, ws)
}

// sweepWorkspace walks every candidate in one workspace, a page at a time.
//
// It walks ALL of them rather than the first page. A candidate that reaches no
// verdict — or one waiting on a human — stays a candidate indefinitely, so a
// pass that only ever read the first page would spend every night on the same
// unresolvable rows while a company behind them, whose corroborated name
// could be applied today, was never reached.
func (p *CompanyNamePromoter) sweepWorkspace(ctx context.Context, ws ids.UUID) error {
	var cursor ids.CompanyID
	for page := 0; page < companyNamePromotionMaxPages; page++ {
		candidates, err := p.store.CompanyNameCandidates(ctx, cursor, companyNamePromotionPageSize)
		if err != nil {
			return err
		}
		for _, cand := range candidates {
			if err := p.decideOne(ctx, cand); err != nil {
				p.log.WarnContext(ctx, "company-name promotion: candidate failed",
					"company", cand.CompanyID.String(), "err", err)
			}
			cursor = cand.CompanyID
		}
		if len(candidates) < companyNamePromotionPageSize {
			return nil
		}
	}
	// Reached only by a workspace far outside any real shape. Said out loud,
	// because a bounded sweep that reports nothing reads exactly like one that
	// covered everything.
	p.log.WarnContext(ctx, "company-name promotion: page ceiling reached, the rest waits for the next pass",
		"workspace", ws.String(), "pages", companyNamePromotionMaxPages)
	return nil
}

func (p *CompanyNamePromoter) decideOne(ctx context.Context, cand people.CompanyNameCandidate) error {
	verdict, ok := people.DecideCompanyName(cand)
	if !ok {
		return nil
	}
	identity, err := companyNameIdentity(cand.CompanyID, verdict.NameKey)
	if err != nil {
		return err
	}
	if verdict.Corroborated {
		return p.applyUnlessDeclined(ctx, cand, verdict)
	}
	// StageUnlessDeclined re-checks under its own lock, so this read only has
	// to keep the inbox tidy: losing the race costs one extra offer, never an
	// unasked write.
	refused, err := p.approvals.RejectedChangesFor(ctx, companyNameProposalKind, cand.CompanyID.UUID)
	if err != nil {
		return err
	}
	if refusedNameKey(refused, verdict.NameKey) {
		return nil
	}
	return p.stageCompanyNameReview(ctx, cand, verdict, identity)
}

// applyUnlessDeclined performs the corroborated rename, refusing if a human has
// already said no to it.
//
// The refusal check and the write share ONE transaction, and the check takes
// the offers' row locks: a human's refusal binds the auto-apply path too, and
// checking in a transaction that commits before the write opens its own would
// leave exactly the gap the check exists to close — reject lands in between,
// and the rename the human refused is applied anyway. Rejecting deliberately
// leaves name_source where it was, so the promotion CAS is no backstop here.
func (p *CompanyNamePromoter) applyUnlessDeclined(ctx context.Context, cand people.CompanyNameCandidate,
	verdict people.CompanyNameVerdict,
) error {
	var promoted bool
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		refused, err := p.approvals.RejectedChangesForTx(ctx, tx, companyNameProposalKind, cand.CompanyID.UUID)
		if err != nil {
			return err
		}
		if refusedNameKey(refused, verdict.NameKey) {
			p.log.InfoContext(ctx, "company-name promotion: name refused by a human, not re-applied",
				"company", cand.CompanyID.String(), "corroboration", verdict.Corroboration)
			return nil
		}
		promoted, err = p.store.PromoteCompanyNameTx(ctx, tx, cand.CompanyID, verdict.Name, verdict.Corroboration)
		return err
	})
	if err != nil {
		return err
	}
	if promoted {
		p.log.InfoContext(ctx, "company-name promotion: corroborated name applied",
			"company", cand.CompanyID.String(), "corroboration", verdict.Corroboration)
	}
	return nil
}

// stageCompanyNameReview offers one uncorroborated name to a human. JoinPending
// keeps a nightly re-run from stacking the same question in the inbox.
func (p *CompanyNamePromoter) stageCompanyNameReview(ctx context.Context, cand people.CompanyNameCandidate,
	verdict people.CompanyNameVerdict, identity json.RawMessage,
) error {
	proposal := companyNameProposal{
		CompanyID:       cand.CompanyID,
		CurrentName:     cand.DisplayName,
		ProposedName:    verdict.Name,
		ProposedNameKey: verdict.NameKey,
		Persons:         verdict.Persons,
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("compose: encoding the company-name proposal: %w", err)
	}
	digest := sha256.Sum256(body)
	// A human who declined this rename declined it for good. StageUnlessDeclined
	// checks and stages in ONE transaction under the approval row lock: checking
	// first and staging afterwards would let a decision land in between, and the
	// refused offer would be recreated anyway — nightly, because the signature
	// that produced it never goes away.
	//
	// Identity, not the diff hash, is what that memory keys on. The payload
	// carries the corroborating persons and the record's current name; both move
	// on their own, and a refusal keyed on the whole payload would be forgotten
	// the first time either did, re-offering the same rename every night until
	// someone clicked approve.
	_, _, err = p.approvals.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           companyNameProposalKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		Identity:       identity,
		TargetType:     companyNameTargetType,
		TargetID:       cand.CompanyID.UUID,
		Summary:        "Rename " + cand.DisplayName + " to " + verdict.Name + "?",
		JoinPending:    true,
	})
	return err
}

// companyNameAcceptEffect builds the approvals.ApprovedEffect for kind
// "company_name_promotion": the human agreed with the single signature, so the
// name is written under the same CAS the corroborated path uses.
//
// There is no reject effect. Rejecting leaves the provisional name exactly
// where it is — the offer only ever renames, so a stale or declined one
// destroys nothing.
func companyNameAcceptEffect(svc *approvals.Service, store *people.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal companyNameProposal
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the company-name proposal: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return fmt.Errorf("compose: company-name accept without a deciding principal")
		}
		// The write carries the machine provenance — the name came from a
		// signature, not from someone typing it — while the human's approval
		// is on the decision's own audit row. That is also why the accepted
		// name stamps name_source='signature' and not 'human': a later human
		// edit must still win over it.
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         companyNamePromotionActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		return svc.RedeemAndApply(ctx, approvalID, companyNameProposalKind, diffHash, func(tx pgx.Tx) error {
			// A false here is the company having been renamed by a
			// stronger source while the offer waited: the approval is spent,
			// nothing is written, and the record keeps the better name.
			_, err := store.PromoteCompanyNameTx(execCtx, tx, proposal.CompanyID,
				proposal.ProposedName, people.CompanyNameCorroborationNone)
			return err
		})
	}
}
