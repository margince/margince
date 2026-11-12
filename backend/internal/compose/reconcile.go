// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Overnight follow-up reconciliation wiring. The deals module owns the nightly
// read pass. This file joins it to the activities store and to approvals.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// followUpStager fills the deals module's FollowUpStager seam. A task goes
// straight onto the deal owner's list; a drafted reply is staged for them,
// because approving it sends mail.
type followUpStager struct {
	svc   *approvals.Service
	tasks *activities.Store
	// draft composes the reply when the evidence is an email thread. Nil is a
	// real configuration rather than a missing one: a role with no send path
	// wired files the task for every candidate.
	draft followUpReplySeam
	// owner resolves the authority the DRAFT is read under. The reply carries a
	// counterparty's address and the message it answers, both of which end up
	// stored on the card — so they are read as the contact the card is for, never
	// under the sweep's own unbounded principal. Its handle is also the
	// transaction the owner is read and the follow-up filed in.
	owner dealOwnerAuthority
	// pool reads the installation's base language the drafted reply's card is
	// written in.
	pool *pgxpool.Pool
}

// HasPendingFollowUp reports a drafted reply still waiting on the deal, so a
// second email cannot stack a second one: approving both would send both. A
// task proposal staged before tasks were filed directly counts too, so its
// deal gets no second task beside it.
func (s followUpStager) HasPendingFollowUp(ctx context.Context, dealID ids.UUID) (bool, error) {
	for _, kind := range []string{deals.FollowUpReconcileKind, automation.HeldDraftKind} {
		pending, err := s.svc.HasPendingKind(ctx, kind, dealID)
		if err != nil {
			return false, err
		}
		if pending {
			return true, nil
		}
	}
	return false, nil
}

func (s followUpStager) StageFollowUp(ctx context.Context, dealID ids.UUID, proposal deals.FollowUpProposal) error {
	// An INBOUND email can be answered, so the rep gets the reply itself rather
	// than a task telling them to write one.
	//
	// Both halves of that condition matter. A call or a meeting has no thread
	// and no address, so a drafted reply to one would be a message to nobody.
	// And OUR OWN last email is not a message anybody is waiting on: the
	// address resolver deliberately answers the counterparty on an outbound
	// message too, so without the direction check the pass drafts a reply to
	// mail the rep sent — and approving it creates another outbound email,
	// which becomes the next night's latest evidence. The deal would then
	// propose a reply every night, forever.
	var draft *followUpDraft
	if s.draft != nil && answerableThread(proposal) {
		composed, ok, err := s.composeDraftedReply(ctx, dealID, proposal)
		if err != nil {
			return err
		}
		if ok {
			draft = &composed
		}
	}
	// The owner is read and the follow-up filed in one transaction, under a lock
	// a reassignment waits behind. So it cannot land with a seat the deal left.
	// The draft was composed before it: a drafting call must not hold the lock.
	return s.owner.db.Tx(ctx, func(tx pgx.Tx) error {
		owner, err := lockedDealOwner(ctx, tx, dealID)
		if err != nil {
			return err
		}
		// A draft composed for someone who no longer owns the deal is text in
		// their name. It is dropped and nothing is staged, so the deal stays
		// eligible and the next sweep drafts for the new owner.
		if draft != nil && draft.owner != owner {
			return nil
		}
		answered, err := followUpAlreadyAnswered(ctx, tx, s.svc, dealID, proposal)
		if err != nil || answered {
			return err
		}
		if draft != nil {
			return stageFollowUpDraft(onBehalfOf(ctx, owner), tx, s.svc, draft.summary,
				dealID, proposal.EvidenceActivityID.UUID, draft.proposal)
		}
		// An unowned deal has nobody to follow it up, so nothing is filed.
		if owner.IsZero() {
			return nil
		}
		return writeOvernightFollowUp(ctx, tx, s.tasks, owner, proposal)
	})
}

// answerableThread reports whether the evidence is a message somebody is
// waiting for a reply to.
func answerableThread(proposal deals.FollowUpProposal) bool {
	return proposal.EvidenceKind == string(crmcontracts.ActivityKindEmail) &&
		proposal.EvidenceDirection == string(crmcontracts.ActivityDirectionInbound)
}

// followUpDraft is a drafted reply composed for one owner, waiting to be
// staged for them.
type followUpDraft struct {
	owner    ids.UUID
	summary  string
	proposal automation.HeldDraftProposal
}

// composeDraftedReply drafts the reply, and reports whether there is one. A
// thread with no answerable counterparty falls back to the task. The rep is
// still told about the deal; they get a task instead of a draft to send.
func (s followUpStager) composeDraftedReply(
	ctx context.Context, dealID ids.UUID, proposal deals.FollowUpProposal,
) (followUpDraft, bool, error) {
	// The draft is composed under the DEAL OWNER's authority, not the sweep's.
	//
	// ReplyAddress resolves the counterparty's email off the contact record,
	// behind an object grant AND a row scope that a system principal walks
	// straight past — auth.Require and auth.ScopeClauseFor both pass it
	// unconditionally. That address is then stored in proposed_change, where
	// the card's own decide grant (activity:create plus deal visibility) is
	// what governs reading it back, and contact:read is not in that set.
	//
	// The message the draft answers is NOT the same question: the reconciler
	// only ever picks evidence with audience = 'workspace', so its subject and
	// body are readable by every decider already. The address has no such
	// filter behind it, which is what makes this the field that needed the
	// owner's authority.
	//
	// A deal nobody owns, or one whose owner is no longer live, gets the TASK
	// instead of a draft. The rep is still told about the deal; there
	// is simply no authority under which a reply may be composed.
	ownerCtx, err := s.owner.contextFor(ctx, dealID)
	if err != nil {
		if errors.Is(err, errNoDealOwner) || errors.Is(err, apperrors.ErrNotFound) {
			return followUpDraft{}, false, nil
		}
		return followUpDraft{}, false, err
	}
	draft, answerable, err := draftFollowUpReply(ownerCtx, s.draft, proposal)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		// The owner may not read what a reply would have to quote. That is the
		// gate working rather than a fault: composing under their authority is
		// exactly what stops a card carrying a read they could not have made.
		// It is also SETTLED — a grant, not an outage — so retrying tomorrow
		// changes nothing, and the rep gets the task instead.
		//
		// draftFollowUpReply itself treats a denial as a failure, correctly: it
		// is handed an authority and cannot know whose. Only here is it known
		// to be the deal owner's, which is what makes a refusal ordinary.
		return followUpDraft{}, false, nil
	}
	if err != nil || !answerable {
		return followUpDraft{}, false, err
	}
	// The draft's own subject is what the rep recognises: it is the thread
	// they are answering, in the words the counterparty used.
	replySummary := fmt.Sprintf(approvalSummaryCopyOver(ctx, s.pool).draftedReplyWaiting, draft.Subject)
	// Filed for exactly the contact whose authority wrote it. The caller stages
	// it only while that contact still owns the deal.
	owner, ok := principal.Actor(ownerCtx)
	if !ok || owner.UserID.IsZero() {
		return followUpDraft{}, false, nil
	}
	return followUpDraft{owner: owner.UserID, summary: replySummary, proposal: draft}, true, nil
}

// onBehalfOf stamps one member as the human a staging acts for, leaving the
// acting principal the sweep's own.
//
// Both halves are load-bearing, which is why this is one function rather than a
// line at each call site. The actor stays the sweep, so the row remains a server
// proposal with a NULL passport that the release executor may run, and its
// provenance stays honest — no rep asked for this card. on_behalf_of names who
// it is FOR, and that is what approvals narrows the decision to.
func onBehalfOf(sweepCtx context.Context, owner ids.UUID) context.Context {
	sweep, ok := principal.Actor(sweepCtx)
	if !ok {
		return sweepCtx
	}
	sweep.OnBehalfOf = owner
	return principal.WithActor(sweepCtx, sweep)
}

// NewFollowUpReconciler assembles the nightly follow-up reconciler for
// the worker process role.
func NewFollowUpReconciler(pool *pgxpool.Pool, log *slog.Logger) *deals.FollowUpReconciler {
	db := InstallationDB(pool)
	// The drafting seam is the same adapter the workflow executors compose
	// through, so an overnight reply and an automation's draft are one drafting
	// engine. The zero SendPath matches that surface: nothing here sends, the
	// held-draft release does, through the fully wired path it builds itself.
	// No system-of-record seam: this adapter is built for DraftEmail alone and
	// its empty SendPath reaches no send, so the guard that would need one is
	// unreachable from here. It refuses rather than passes if that ever changes.
	drafter := newCommsAdapter(pool, nil, SendPath{})
	stager := followUpStager{
		svc:   NewApprovalsService(db),
		tasks: activities.NewStore(db),
		draft: drafter,
		owner: dealOwnerAuthority{db: db, users: identity.NewServiceFor(db)},
		pool:  pool,
	}
	return deals.NewFollowUpReconciler(db, stager, log).WithBaseLanguage(installationLanguage(pool))
}

// followUpPrecheck refuses a DECISION whose payload the effect could not use.
//
// It exists because the two halves are not one transaction: the approval
// commits, then the effect runs, and a failed effect never un-decides it. So a
// payload the effect chokes on produces an approved row nothing can decide
// again and no surface can re-drive — the rep sees their yes recorded, no task
// appears, and nothing says why. Refusing BEFORE the decision leaves the row
// pending, which is the state a human can act on: fix the date, approve again.
//
// The due date is the reachable case rather than a theoretical one. The card
// lets a human edit the proposal, and this is the field the effect parses.
//
// It checks the payload about to be committed — the edit when there is one, the
// staged proposal otherwise — because those are different documents and only
// one of them is what the effect will read.
//
// The refusal is approvals' own InvalidEditError so it lands on 422 naming the
// field, the way the send precheck's refusals already do. An untyped error here
// reaches the generic mapper and reads as 500 internal, which tells the rep
// their approval broke the server rather than that the date needs fixing —
// leaving them nothing to act on and no reason to try again.
func followUpPrecheck() approvals.ReleasePrecheck {
	return func(_ context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		proposal, err := deals.UnmarshalFollowUpProposal(payload)
		if err != nil {
			return &approvals.InvalidEditError{Cause: err}
		}
		if _, err := time.Parse(time.DateOnly, proposal.DueDate); err != nil {
			return &approvals.InvalidEditError{Cause: fmt.Errorf(
				"the due date %q is not a date — write it as YYYY-MM-DD", proposal.DueDate)}
		}
		return nil
	}
}

// followUpConfirmEffect executes a follow-up proposal staged before the pass
// filed its tasks directly. It redeems, then creates, like every 🟡 executor.
// The task goes through the activities store, the RBAC-gated write a rep's own
// "add task" takes. The single-use redemption claims it once, and the write is
// idempotent on (source_system, source_id) keyed to the approval. So a
// re-driven decision creates nothing twice. It runs as the overnight agent on
// behalf of the deciding human, who is on the decision's own audit row.
func followUpConfirmEffect(svc *approvals.Service, store *activities.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		proposal, err := deals.UnmarshalFollowUpProposal(proposedChange)
		if err != nil {
			return err
		}
		staged, err := svc.Get(ctx, approvalID)
		if err != nil {
			return err
		}
		metadata := approvals.EffectSystemText(staged.SummaryParts)

		task, err := followUpTaskInput(metadata, proposal, approvalID.String())
		if err != nil {
			return err
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: follow-up effect without a deciding principal")
		}
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         overnightAgent,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		// Redemption and the write in ONE transaction. Consuming the approval
		// first and writing after leaves the two able to disagree: the approval
		// reads as redeemed while no task exists, and the audit row asserts a
		// redemption for a write that never landed. Here a failed write rolls
		// the redemption back with it, so redeemed and created stay the same
		// fact.
		//
		// The rollback also keeps a failed effect re-drivable: the approval is
		// still unconsumed, so deciding it again runs this effect again
		// (approvals.effectIsStillOwed).
		return svc.RedeemAndApply(ctx, approvalID, deals.FollowUpReconcileKind, diffHash,
			func(tx pgx.Tx) error {
				_, _, err := store.LogActivityTx(execCtx, tx, task)
				return err
			})
	}
}
