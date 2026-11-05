// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// siteLeadNoticeReviewed is the privacy_notice version legal review approved
// for website leads; zero means none has passed. Setting it opens the lane.
//
// Held by: TestSiteLeadCaptureOpensOnlyWithAReviewedNotice
// (backend/internal/compose/siteleadnotice_test.go)
const siteLeadNoticeReviewed = 0

// siteLeadLaneRefusal answers why somebody a company's website names may not
// become a lead, or nil when they may. The lane is open only when a reviewed
// version is named and it is the version this build sends.
//
// Every path asks this one function: staging, the precheck, and the accept
// step, which a re-driven decision reaches without the precheck.
func siteLeadLaneRefusal(reviewed int) error {
	if reviewed == 0 {
		return errSiteLeadCaptureClosed
	}
	if version, _ := consent.ControllerTemplateVersion(consent.TemplatePrivacyNotice); version != reviewed {
		return errSiteLeadNoticeUnreviewed
	}
	return nil
}

// errSiteLeadCaptureClosed is what accepting a site lead answers while the lane
// is shut. A conflict rather than a denial: the decider may accept proposals,
// and this one cannot be carried out until the notice has been reviewed.
var errSiteLeadCaptureClosed = fmt.Errorf("%w: somebody named on a website cannot be captured "+
	"until the privacy notice they are sent has passed legal review; the proposal stays in the inbox",
	apperrors.ErrConflict)

// errSiteLeadNoticeUnreviewed refuses while the wording this build sends is not
// the version the review approved.
var errSiteLeadNoticeUnreviewed = fmt.Errorf("%w: the privacy notice wording changed after its "+
	"legal review, so somebody named on a website cannot be captured until it is reviewed again",
	apperrors.ErrConflict)

// errSiteLeadHasNoAddress answers an accept of somebody whose page published no
// address: they cannot be told, so they are not kept.
var errSiteLeadHasNoAddress = fmt.Errorf("%w: the page published no address to send the privacy "+
	"notice to, so this contact was not kept", apperrors.ErrConflict)

// siteLeadNoticeLane is the consent store a website lead is told through.
//
// Filled by rewireConfirmationLane, which runs after the approvals service is
// built, so the accept holds this pointer rather than a store. Empty means the
// installation cannot send its own mail, and every site lead is refused.
type siteLeadNoticeLane struct {
	store atomic.Pointer[consent.Store]
	// reviewed is the reviewed wording version this lane may send. Zero, the
	// value of a lane nobody built with newSiteLeadNoticeLane, keeps it shut.
	reviewed int
}

// newSiteLeadNoticeLane is the lane production builds, opened by the review.
func newSiteLeadNoticeLane() *siteLeadNoticeLane {
	return &siteLeadNoticeLane{reviewed: siteLeadNoticeReviewed}
}

// refusal is siteLeadLaneRefusal for this lane.
func (l *siteLeadNoticeLane) refusal() error {
	if l == nil {
		return errSiteLeadCaptureClosed
	}
	return siteLeadLaneRefusal(l.reviewed)
}

// teller is the store, when the lane is wired.
func (l *siteLeadNoticeLane) teller() (*consent.Store, bool) {
	if l == nil {
		return nil, false
	}
	store := l.store.Load()
	return store, store != nil && store.CanTellLeads()
}

// siteLeadPrecheck refuses accepting a site lead before the decision commits.
// The proposal then stays pending instead of becoming an approval nothing
// carries out.
func siteLeadPrecheck(notices *siteLeadNoticeLane) approvals.ReleasePrecheck {
	return func(context.Context, json.RawMessage, json.RawMessage) error {
		if err := notices.refusal(); err != nil {
			return err
		}
		if _, ok := notices.teller(); !ok {
			return consent.ErrNoNoticeLane
		}
		return nil
	}
}

// siteLeadSteps are the accept's two writes the notice step runs in its own
// transaction: redeeming the approval, and capturing the contact as a lead.
// capture answers false when the address belongs to another source's lead.
type siteLeadSteps struct {
	redeem  func(ctx context.Context) error
	capture siteLeadCaptureStep
}

// siteLeadCaptureStep is the capture half of siteLeadSteps.
type siteLeadCaptureStep = func(ctx context.Context) (ids.LeadID, bool, error)

// untoldReason is why a website lead's notice was not given, and the error
// the decider is answered with. A zero reason means the notice was staged.
type untoldReason struct {
	reason string
	cause  error
}

// errSiteLeadCollision answers an accept whose published address another
// source's lead already holds: the website's copy is not kept.
var errSiteLeadCollision = fmt.Errorf("%w: a lead from another source already holds this address, "+
	"so the website's copy of this contact was not kept", apperrors.ErrConflict)

// errSiteLeadIsAContact answers an accept for somebody who is already a
// contact: no lead is kept and no notice is sent. The notice record names it a
// collision, the address being another record's.
var errSiteLeadIsAContact = fmt.Errorf("%w: the address is already a contact's, "+
	"so no lead was kept and no notice was sent", apperrors.ErrConflict)

// sendArticle14Notice keeps a website lead only with the notice it is owed.
// One transaction holds the redemption, the capture, the staged notice, and,
// when the notice cannot be given, the record of why.
//
// A notice that cannot be given keeps nothing. The capture is rolled back to a
// savepoint, the proposal is emptied, and a record naming nobody says why. A
// bounce later ends the same way (capturebounce.go).
//
// A failure of the transaction itself commits nothing, the redemption
// included, so deciding the approval again re-drives the whole step.
func sendArticle14Notice(
	ctx context.Context, pool *pgxpool.Pool, notices *siteLeadNoticeLane,
	approvalID ids.ApprovalID, proposal siteLeadProposal, steps siteLeadSteps,
) error {
	var cause error
	err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		if err := steps.redeem(ctx); err != nil {
			return err
		}
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			untold, err := keepOnlyIfTold(ctx, tx, notices, approvalID, proposal, steps.capture)
			if err != nil || untold.reason == "" {
				return err
			}
			cause = untold.cause
			return forgetUntoldSiteLeadTx(ctx, tx, approvalID, untold.reason)
		})
	})
	if err != nil {
		return err
	}
	return cause
}

// keepOnlyIfTold captures the lead and stages its notice behind a savepoint.
// It answers why when the notice cannot be given, with the savepoint rolled
// back, and an error only when the transaction itself broke.
func keepOnlyIfTold(
	ctx context.Context, tx pgx.Tx, notices *siteLeadNoticeLane,
	approvalID ids.ApprovalID, proposal siteLeadProposal, capture siteLeadCaptureStep,
) (untoldReason, error) {
	if strings.TrimSpace(proposal.PublishedEmail) == "" {
		return untoldReason{consent.LeadNoticeNoAddress, errSiteLeadHasNoAddress}, nil
	}
	teller, ok := notices.teller()
	if !ok {
		return untoldReason{consent.LeadNoticeRefused, consent.ErrNoNoticeLane}, nil
	}
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return untoldReason{}, fmt.Errorf("compose: savepoint before keeping a website lead: %w", err)
	}
	untold := tellAndKeep(ctx, teller, approvalID, capture)
	if untold.reason == "" {
		return untoldReason{}, savepoint.Commit(ctx)
	}
	if err := savepoint.Rollback(ctx); err != nil {
		return untoldReason{}, fmt.Errorf("compose: rolling back a website lead that was not told: %w", err)
	}
	return untold, nil
}

// tellAndKeep runs the capture and the notice, and names why it failed.
func tellAndKeep(
	ctx context.Context, teller *consent.Store, approvalID ids.ApprovalID, capture siteLeadCaptureStep,
) untoldReason {
	leadID, ours, err := capture(ctx)
	if errors.Is(err, errSiteLeadIsAContact) {
		return untoldReason{consent.LeadNoticeCollision, err}
	}
	if err != nil {
		return untoldReason{consent.LeadNoticeRefused, err}
	}
	if !ours {
		return untoldReason{consent.LeadNoticeCollision, errSiteLeadCollision}
	}
	if _, err := teller.IssueLeadPrivacyNotice(ctx, consent.LeadNotice{LeadID: leadID, ApprovalID: approvalID}); err != nil {
		return untoldReason{consent.LeadNoticeRefused, err}
	}
	return untoldReason{}
}

// forgetUntoldSiteLeadTx records why a website lead's notice was not given and
// empties what carried their details, on the accept's own transaction.
func forgetUntoldSiteLeadTx(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, failure string) error {
	if err := consent.RecordLeadNoticeFailedTx(ctx, tx, approvalID, failure); err != nil {
		return err
	}
	return privacy.EraseUntoldLeadTx(ctx, tx, privacy.UntoldLead{ApprovalID: approvalID})
}

// siteLeadsRefused is the one staging-side check, asked by every path that would
// stage names a website published: the crawl worker's and the onboarding confirmation's. It
// reports true, and says why in the log, when the lane is shut and there was
// somebody to refuse.
func siteLeadsRefused(ctx context.Context, log *slog.Logger, readID string, found int) bool {
	if found == 0 || siteLeadLaneRefusal(siteLeadNoticeReviewed) == nil {
		return false
	}
	log.InfoContext(ctx, "published contacts not proposed",
		"lane", laneContacts, "reason", dropNoArticle14Notice, "read", readID, "contacts", found)
	return true
}
