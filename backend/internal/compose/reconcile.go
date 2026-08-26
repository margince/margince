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
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/diffhash"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
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
}

func (s followUpStager) HasPendingFollowUp(ctx context.Context, dealID ids.UUID) (bool, error) {
	return s.svc.HasPendingKind(ctx, deals.FollowUpReconcileKind, dealID)
}

func (s followUpStager) StageFollowUp(ctx context.Context, dealID ids.UUID, summary string, proposal deals.FollowUpProposal) error {
	raw, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("compose: marshal follow-up proposal: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return fmt.Errorf("compose: canonicalize follow-up proposal: %w", err)
	}
	// The logical identity is the DEAL, and nothing else.
	//
	// It must not include the due date. The proposal's date moves with "today",
	// so an identity carrying it makes every night's proposal a different one —
	// and StageUnlessDeclined, which recognises a proposal a human already
	// declined, would recognise nothing. A rep's "no" then became a question
	// they were asked again the following morning.
	//
	// Identity fields must appear in ProposedChange with the same value
	// (canonicalIdentity enforces it), and deal_id does: the payload carries it
	// as the deal this follow-up is for.
	identity, err := json.Marshal(map[string]string{"deal_id": dealID.String()})
	if err != nil {
		return fmt.Errorf("compose: marshal follow-up identity: %w", err)
	}
	_, _, err = s.svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           deals.FollowUpReconcileKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     "deal",
		TargetID:       dealID,
		Summary:        summary,
		Identity:       identity,
		JoinPending:    true,
	})
	return err
}

// NewFollowUpReconciler assembles the nightly follow-up reconciler for
// the worker process role.
func NewFollowUpReconciler(pool *pgxpool.Pool, log *slog.Logger) *deals.FollowUpReconciler {
	return deals.NewFollowUpReconciler(InstallationDB(pool), followUpStager{svc: approvals.NewService(InstallationDB(pool))}, log)
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
func followUpPrecheck() approvals.ReleasePrecheck {
	return func(_ context.Context, staged, edited json.RawMessage) error {
		payload := staged
		if len(edited) > 0 {
			payload = edited
		}
		proposal, err := deals.UnmarshalFollowUpProposal(payload)
		if err != nil {
			return fmt.Errorf("this follow-up cannot be created as written: %w", err)
		}
		if _, err := time.Parse(time.DateOnly, proposal.DueDate); err != nil {
			return fmt.Errorf(
				"the due date %q is not a date — write it as YYYY-MM-DD", proposal.DueDate)
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
		// Redemption and the write in ONE transaction, which is what makes a
		// failure recoverable. Consuming the approval first and writing after
		// means a failed write spends the human's yes and leaves no task and no
		// way to try again — the approval is decided, redeemed, and no surface
		// can re-drive it. Here a failed write rolls the redemption back with
		// it, so the decision stays releasable.
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
