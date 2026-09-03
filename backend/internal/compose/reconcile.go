// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Overnight follow-up reconciliation wiring (features/07 §8a,
// B-E06.2a): the deals module owns the nightly read pass and the
// approvals module owns the 🟡 morning inbox — this file is the
// cross-module edge between them, injected here like every other one.
// The pass stages kind "deal_follow_up" through the adapter below; a
// human approval releases the confirm effect, which redeems the staging
// and creates the drafted follow-up task through the activities store's
// own gated write path.

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
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// followUpStager adapts the approvals service onto the deals module's
// FollowUpStager seam. The proposal carries no target_version: a
// follow-up is a new activity, independent of the deal's current field
// values, so a concurrent deal edit must not invalidate the human's yes
// (unlike a close-date correction, which overwrites a deal field).
//
// Leaving TargetVersion unset is NOT what makes that true, which is worth
// saying because it used to be. The pin is taken server-side now, at the one
// place every stager passes through, so a stager cannot decline it by
// omission — and for a while this comment described an intention the code had
// silently stopped honouring. What declines it is the kind's entry in
// approvals' contextTargetKinds, and this paragraph is accurate exactly as
// long as that entry exists.
type followUpStager struct {
	svc *approvals.Service
	// draft composes the reply when the evidence is an email thread. Nil is a
	// real configuration rather than a missing one: a role with no send path
	// wired stages the task proposal for every candidate, which is what the
	// pass did before the drafted reply existed.
	draft followUpReplySeam
	// owner resolves the authority the DRAFT is read under. The reply carries a
	// counterparty's address and the message it answers, both of which end up
	// stored on the card — so they are read as the person the card is for, never
	// under the sweep's own unbounded principal.
	owner dealOwnerAuthority
}

// HasPendingFollowUp answers for BOTH shapes a follow-up can take.
//
// The pass proposes either a task or a drafted reply, and they are different
// approval kinds. Asking only about the task let a second email stack a second
// drafted reply on a deal whose first was still pending — two independently
// approvable messages, and approving both sends both. The question the caller
// is really asking is "has this deal already been asked about", which neither
// kind answers alone.
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

func (s followUpStager) StageFollowUp(ctx context.Context, dealID ids.UUID, summary string, proposal deals.FollowUpProposal) error {
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
	if s.draft != nil && answerableThread(proposal) {
		staged, err := s.stageDraftedReply(ctx, dealID, proposal)
		if err != nil {
			return err
		}
		if staged {
			return nil
		}
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("compose: marshal follow-up proposal: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return fmt.Errorf("compose: canonicalize follow-up proposal: %w", err)
	}
	// The logical identity is the deal AND the interaction the proposal was
	// drawn from — the two fields that say WHICH follow-up this is.
	//
	// It must not include the due date. The proposal's date moves with "today",
	// so an identity carrying it makes every night's proposal a different one —
	// and StageUnlessDeclined, which recognises a proposal a human already
	// declined, would recognise nothing. A rep's "no" then became a question
	// they were asked again the following morning.
	//
	// It must not be the deal ALONE either, for the opposite reason. A decline
	// is remembered with no expiry, so a deal-only identity lets one "no" bury
	// every future follow-up on that deal: the rep declines after a discovery
	// call, has a real conversation three weeks later that again ends with no
	// next step, and is never asked. The evidence activity is what separates
	// those two cases — the nightly pass picks the LATEST interaction, so it is
	// the same value while the situation is unchanged and a new one exactly when
	// the deal has genuinely moved.
	//
	// Identity fields must appear in ProposedChange with the same value
	// (canonicalIdentity enforces it), and both do: the payload carries the deal
	// this follow-up is for and the interaction that triggered it.
	identity, err := json.Marshal(map[string]string{
		"deal_id":              dealID.String(),
		"evidence_activity_id": proposal.EvidenceActivityID.String(),
	})
	if err != nil {
		return fmt.Errorf("compose: marshal follow-up identity: %w", err)
	}
	_, _, err = s.svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           deals.FollowUpReconcileKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     approvalTargetDeal,
		TargetID:       dealID,
		Summary:        summary,
		Identity:       identity,
		JoinPending:    true,
	})
	return err
}

// answerableThread reports whether the evidence is a message somebody is
// waiting for a reply to.
func answerableThread(proposal deals.FollowUpProposal) bool {
	return proposal.EvidenceKind == string(crmcontracts.ActivityKindEmail) &&
		proposal.EvidenceDirection == string(crmcontracts.ActivityDirectionInbound)
}

// stageDraftedReply offers the drafted reply, and reports whether it was
// taken. A thread with no answerable counterparty falls back to the task
// proposal rather than dropping the candidate — the rep is still told about
// the deal, they simply get "write a follow-up" instead of a draft to send.
func (s followUpStager) stageDraftedReply(
	ctx context.Context, dealID ids.UUID, proposal deals.FollowUpProposal,
) (bool, error) {
	// The draft is composed under the DEAL OWNER's authority, not the sweep's.
	//
	// ReplyAddress resolves the counterparty's email off the person record,
	// behind an object grant AND a row scope that a system principal walks
	// straight past — auth.Require and auth.ScopeClauseFor both pass it
	// unconditionally. That address is then stored in proposed_change, where
	// the card's own decide grant (activity:create plus deal visibility) is
	// what governs reading it back, and person:read is not in that set.
	//
	// The message the draft answers is NOT the same question: the reconciler
	// only ever picks evidence with audience = 'workspace', so its subject and
	// body are readable by every decider already. The address has no such
	// filter behind it, which is what makes this the field that needed the
	// owner's authority.
	//
	// A deal nobody owns, or one whose owner is no longer live, gets the TASK
	// proposal instead of a draft. The rep is still told about the deal; there
	// is simply no authority under which a reply may be composed.
	ownerCtx, err := s.owner.contextFor(ctx, dealID)
	if err != nil {
		if errors.Is(err, errNoDealOwner) || errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	draft, answerable, err := draftFollowUpReply(ownerCtx, s.draft, proposal)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		// The owner may not read what a reply would have to quote. That is the
		// gate working rather than a fault: composing under their authority is
		// exactly what stops a card carrying a read they could not have made.
		// It is also SETTLED — a grant, not an outage — so retrying tomorrow
		// changes nothing, and the rep gets the task proposal instead.
		//
		// draftFollowUpReply itself treats a denial as a failure, correctly: it
		// is handed an authority and cannot know whose. Only here is it known
		// to be the deal owner's, which is what makes a refusal ordinary.
		return false, nil
	}
	if err != nil || !answerable {
		return false, err
	}
	// Its OWN summary rather than the task proposal's with a clause appended.
	// The task's line names a task ("Draft a follow-up on …"), and a reply
	// waiting to be sent is a different thing to tell a rep — appending to it
	// produced a sentence that described both and read as neither.
	//
	// The draft's own subject is what the rep recognises: it is the thread
	// they are answering, in the words the counterparty used.
	replySummary := fmt.Sprintf("A reply to %q is drafted and waiting to be sent — "+
		"the conversation left no next step planned", draft.Subject)
	// Staged as the SWEEP, recording the OWNER as the human it acts for.
	//
	// The two halves answer different questions and both are load-bearing. The
	// acting principal stays the sweep's, which is what keeps the row a server
	// proposal (a NULL passport) the release executor may run, and what keeps
	// its provenance honest — no rep asked for this card. on_behalf_of names
	// the person the card is FOR, and approvals narrows a held draft to them:
	// releasing one sends it from the approver's own mailbox, so a colleague
	// who released it would be answering a customer under their own name.
	//
	// Recorded under the sweep alone, as it was, the row named nobody — and a
	// held draft naming nobody is decidable by nobody.
	if err := stageFollowUpDraft(onBehalfOfOwner(ctx, ownerCtx), s.svc, replySummary,
		dealID, proposal.EvidenceActivityID.UUID, draft); err != nil {
		return false, err
	}
	return true, nil
}

// onBehalfOfOwner returns the sweep's own principal, recording the owner it
// resolved as the human this staging acts for.
//
// The owner is taken from the context the draft was composed under rather than
// re-read, so the card is filed for exactly the person whose authority wrote it.
// A context carrying no human leaves the principal untouched: the staging then
// records nobody, which is what an ownerless deal honestly is.
func onBehalfOfOwner(sweepCtx, ownerCtx context.Context) context.Context {
	owner, ok := principal.Actor(ownerCtx)
	if !ok || owner.UserID.IsZero() {
		return sweepCtx
	}
	sweep, ok := principal.Actor(sweepCtx)
	if !ok {
		return sweepCtx
	}
	sweep.OnBehalfOf = owner.UserID
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
	drafter := newCommsAdapter(pool, nil, SendPath{})
	stager := followUpStager{
		svc:   approvals.NewService(db),
		draft: drafter,
		owner: dealOwnerAuthority{db: db, users: identity.NewServiceFor(db)},
	}
	return deals.NewFollowUpReconciler(db, stager, log)
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

// followUpConfirmEffect executes an approved follow-up: redeem-then-
// create like every 🟡 executor, then log the drafted (possibly human-
// edited) follow-up task through the activities store — the same
// RBAC-gated write a rep's own "add task" takes. The single-use
// redemption is the exactly-once claim, and the write is additionally
// idempotent on (source_system, source_id) keyed to the approval, so a
// re-driven decision creates nothing twice. It runs as the overnight
// agent on behalf of the deciding human: captured_by=agent:overnight
// (the follow-up is the agent's suggestion), the human is on the
// decision's own audit row.
func followUpConfirmEffect(svc *approvals.Service, store *activities.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		proposal, err := deals.UnmarshalFollowUpProposal(proposedChange)
		if err != nil {
			return err
		}
		due, err := time.Parse(time.DateOnly, proposal.DueDate)
		if err != nil {
			return fmt.Errorf("compose: follow-up due date: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return fmt.Errorf("compose: follow-up effect without a deciding principal")
		}
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         "agent:overnight",
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		subject := proposal.Subject
		body := proposal.Body
		sourceSystem := "overnight-reconcile"
		sourceID := approvalID.String()
		// Redemption and the write in ONE transaction. Consuming the approval
		// first and writing after leaves the two able to disagree: the approval
		// reads as redeemed while no task exists, and the audit row asserts a
		// redemption for a write that never landed. Here a failed write rolls
		// the redemption back with it, so redeemed and created stay the same
		// fact.
		//
		// It does NOT make a failed effect re-drivable. The decision commits
		// before any effect runs, so a failure here leaves an approved row with
		// its redemption intact and nothing to re-trigger it — deciding again
		// returns AlreadyDecided. Closing that needs the effect inside the
		// decision transaction, or a durable retry, and it is the approvals
		// module's to close for every kind rather than this one's.
		return svc.RedeemAndApply(ctx, approvalID, deals.FollowUpReconcileKind, diffHash,
			func(tx pgx.Tx) error {
				_, _, err := store.LogActivityTx(execCtx, tx, activities.LogActivityInput{
					Kind:         "task",
					Subject:      &subject,
					Body:         &body,
					DueAt:        &due,
					SourceSystem: &sourceSystem,
					SourceID:     &sourceID,
					Source:       "overnight-reconcile",
					Links:        []activities.ActivityLinkInput{{EntityType: "deal", EntityID: proposal.DealID.UUID}},
				})
				return err
			})
	}
}
