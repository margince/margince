// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The coldstart ACCEPT executor (features/07 §1): a human approval of a
// staged read-back now WRITES the accepted fields onto the company
// the source URL names — the follow-on effect that closes the
// stage→approve loop. Redeem-then-execute like every 🟡 executor: the
// single-use redemption is the exactly-once claim, so a replayed or
// re-driven decision applies nothing twice.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// approvalsHandlersWithEffects wires the approvals HTTP surface with
// every registered follow-on effect — the decision path and the effects
// share one service so a released effect can redeem what it decides on.
//
// volume is the meter an approved step-up widens, and it is installed HERE
// and nowhere else: this is the service the HTTP decision path runs on. The
// other services built from approvalsServiceWithEffects are staging-only — a
// nightly proposer, a rematch sweep — and none of them ever decides, so having
// no releaser is the shape of what they do rather than a copy-paste omission.
//
// log is installed for the same reason and on the same service: a bundle member
// whose effect fails AFTER its decision committed is reported to the client by
// outcome alone, so this is the process that has to carry the cause.
// It also answers the lane a website lead is told through, installed here
// alone, for rewireConfirmationLane to fill once the mail lane is wired.
func approvalsHandlersWithEffects(
	pool *pgxpool.Pool, volume approvals.VolumeReleaser, log *slog.Logger,
) (approvals.Handlers, *siteLeadNoticeLane) {
	notices := newSiteLeadNoticeLane()
	return approvals.NewHandlers(approvalsServiceTelling(pool, notices).WithVolumeReleaser(volume).WithLogger(log)), notices
}

// approvalsServiceWithEffects is the registration list itself, split from the
// handler wiring so a fitness test can enumerate what is stageable without
// standing up an HTTP surface. Every kind registered here must carry a
// decision-grant mapping (TestEveryRegisteredEffectKindHasADecisionGrantMapping).
func approvalsServiceWithEffects(pool *pgxpool.Pool) *approvals.Service {
	return approvalsServiceTelling(pool, &siteLeadNoticeLane{})
}

// approvalsServiceTelling is approvalsServiceWithEffects with the lane a website
// lead is told through. An empty lane refuses every site lead, which is right
// for the services that only stage.
func approvalsServiceTelling(pool *pgxpool.Pool, notices *siteLeadNoticeLane) *approvals.Service {
	captureConfig := CaptureConfig{}
	if loaded := config.Bound(); loaded != nil {
		captureConfig = CaptureConfigFromDeploy(loaded, slog.Default())
	}
	svc := NewApprovalsService(InstallationDB(pool)).
		WithUndoableRelease(undoableAgentRelease(activities.FilingsStayUndoable))
	store := newCounterpartyStore(pool)
	svc.WithEffect("coldstart", coldstartAcceptEffect(svc, store))
	svc.WithEffect(enrichProposalKind, scrapeAcceptEffect(svc, store))
	svc.WithEffect(deepReadProposalKind, deepReadAcceptEffect(svc, store))
	svc.WithEffect(siteLeadProposalKind,
		siteLeadAcceptEffect(svc, newCaptureSink(pool, captureConfig), pool, notices))
	svc.WithPrecheck(siteLeadProposalKind, siteLeadPrecheck(notices))
	svc.WithEffect(counterpartyProposalKind, counterpartyAcceptEffect(svc, store, newConnectorTagFiler(pool), capture.NewPendingStore(InstallationDB(pool)), newDomainTriageTrigger(pool, slog.Default(), captureConfig.SkipReservedDomainProposals)))
	svc.WithPrecheck(counterpartyProposalKind, counterpartyPrecheck(store))
	svc.WithEffect(companyNameProposalKind, companyNameAcceptEffect(svc, store))
	svc.WithEffect(captureCollisionKind, captureCollisionAcceptEffect(svc, store))
	svc.WithEffect(linkedInMatchKind, linkedInMatchAcceptEffect(svc, store))
	svc.WithPrecheck(linkedInMatchKind, linkedInMatchPrecheck(store))
	// Both halves, like the held message above and for the same reason: the
	// subject of this card is a row that is already sitting in a state, and a
	// card whose buttons only dismissed it would report a decision the record
	// never heard.
	svc.WithDeclinedEffect(linkedInMatchKind, linkedInMatchDeclineEffect(store))
	svc.WithEffect(lifecycleProposalKind, lifecycleAcceptEffect(svc, store, time.Now))
	svc.WithEffect(vcardCreateKind, vcardCreateAcceptEffect(svc,
		newContactsStore(InstallationDB(pool)).WithShareRequests(contactShareRequests(pool))))
	svc.WithEffect(contactShareKind, contactShareAcceptEffect(svc, identity.NewServiceFor(InstallationDB(pool))))
	// Both halves: the colleague who asked is told how their request ended.
	transfers := newOwnershipTransfers(pool)
	svc.WithEffect(ownershipTransferKind, ownershipTransferAcceptEffect(svc, transfers))
	svc.WithPrecheck(ownershipTransferKind, transfers.transferPrecheck())
	svc.WithDeclinedEffect(ownershipTransferKind, declineTransferEffect())
	svc.WithPrecheck(vcardCreateKind, vcardCreatePrecheck())
	// A held message is the one kind with BOTH halves registered, because its
	// subject is already waiting: Accept re-arms it, Reject abandons it, and a
	// card whose buttons only dismissed it would report a decision the message
	// never heard (#1312, ADR-0104 §5).
	if sendStore, timer, ok := heldSendActors(pool); ok {
		svc.WithEffect(heldScheduledSendKind, heldAcceptEffect(svc, sendStore, timer))
		svc.WithDeclinedEffect(heldScheduledSendKind, heldDeclineEffect(sendStore))
	}
	// The provider is rebuilt exactly as workflows.go builds the one an
	// automation writes through: a released reassignment must reach the same
	// record surface reassigning a single record does.
	svc.WithEffect(string(workflow.ActionAssignOwner), assignOwnerReleaseEffect(svc,
		NewProvider(pool), InstallationDB(pool)))
	svc.WithEffect(deals.CloseDateCorrectionKind, closeDateConfirmEffect(svc, deals.NewStore(InstallationDB(pool), DealsInstallation())))
	svc.WithEffect(deals.FollowUpReconcileKind, followUpConfirmEffect(svc, activities.NewStore(InstallationDB(pool))))
	svc.WithPrecheck(deals.FollowUpReconcileKind, followUpPrecheck())
	commitmentTasks := activities.NewStore(InstallationDB(pool))
	svc.WithEffect(CommitmentTaskKind, commitmentTaskEffect(svc, commitmentTasks, newContactsStore(InstallationDB(pool))))
	svc.WithPrecheck(CommitmentTaskKind, commitmentTaskPrecheck(InstallationDB(pool), commitmentTasks))
	svc.WithDeclinedEffect(CommitmentTaskKind, commitmentTaskDeclineEffect())
	svc.WithEffect(deals.StageProgressionKind, stageProgressionEffect(svc,
		deals.NewStore(InstallationDB(pool), DealsInstallation()).
			WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))})))
	svc.WithPrecheck(deals.StageProgressionKind, stageProgressionPrecheck())
	svc.WithEffect(fxRateProposalKind, fxRateAcceptEffect(svc, deals.NewStore(InstallationDB(pool), DealsInstallation())))
	return svc
}

// expiringApprovalsService is the engine the clock's sweep runs on: every
// expiry effect a kind registers, and nothing else. Split from the deciding
// registration because the two services run in different processes — the
// sweep is a worker job — and a hook registered only on the deciding one
// would be absent exactly where the expiry happens.
func expiringApprovalsService(pool *pgxpool.Pool) *approvals.Service {
	return NewApprovalsService(InstallationDB(pool)).
		WithExpiredEffect(ownershipTransferKind, expireTransferEffect())
}

// coldstartAcceptEffect builds the approvals.ApprovedEffect compose
// injects for kind "coldstart".
func coldstartAcceptEffect(svc *approvals.Service, store *contacts.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		sourceURL, fields, err := contacts.UnmarshalColdStartFields(proposedChange)
		if err != nil {
			return err
		}
		// The write executes as the coldstart executor: captured_by =
		// agent:coldstart / source = coldstart (features/07 §1 AC), on
		// behalf of the human whose approval released it — that human is
		// on the decision's own audit row, this one carries the machine
		// provenance the 360 renders as "read from your site".
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: coldstart effect without a deciding principal")
		}
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         "agent:coldstart",
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		// The single-use redemption IS the idempotency claim: whoever consumes
		// the approval executes; anyone else finds it consumed. Redeemed in the
		// write's OWN transaction, so a failed apply leaves the approval
		// unconsumed and retryable — spent first, a write that then fails loses
		// the change with no path back.
		//
		// Redeemed under the DECIDER's context and applied under the executor's:
		// the redemption asserts that human's authority, the write carries the
		// machine provenance.
		return svc.RedeemAndApply(ctx, approvalID, "coldstart", diffHash, func(tx pgx.Tx) error {
			_, err := store.ApplyColdStartProfileTx(execCtx, tx, contacts.ApplyColdStartProfileInput{
				SourceURL: sourceURL,
				Fields:    fields,
			})
			return err
		})
	}
}
