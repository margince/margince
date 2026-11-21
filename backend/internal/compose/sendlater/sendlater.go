// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sendlater holds what Send anyway shares between sending now and
// sending later. That is the checks before any attempt, and the refusal when
// the engine's answer moved since the sender read the review. The later path
// lives here too. It records the decision and moves the held message in one
// transaction, and the fire at the moment spends the decision.
package sendlater

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/sendmetrics"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Service holds what a Send anyway spans: two modules, the engine, the
// delivery machinery, and the transaction both writes share.
type Service struct {
	Consent  *consent.Store
	Sends    *activities.Store
	Gate     *consent.Gate
	Delivery activities.DeliveryStager
	Timer    activities.ScheduleTimer
	DB       *database.DB
	// Actions is what the caller may do about a refreshed review.
	Actions func(context.Context, consent.Review) []string
}

// Admitted is a Send anyway that passed every check before any attempt.
type Admitted struct {
	Intent ids.UUID
	// Read is the review as the sender read it.
	Read consent.Review
	In   consent.DirectInput
}

// DecisionOf reads the sender's decision out of a Send anyway request.
func DecisionOf(body crmcontracts.DirectCommunicationSendRequest) consent.DirectInput {
	in := consent.DirectInput{
		ReasonCode:     string(body.ReasonCode),
		Explanation:    body.Explanation,
		WarningVersion: body.WarningVersion,
		Acknowledged:   body.Acknowledged,
	}
	if body.AssessmentRevision != nil {
		in.AssessmentRevision = *body.AssessmentRevision
	}
	if body.ShownReasons != nil {
		in.ShownReasons = *body.ShownReasons
	}
	return in
}

// Admit runs every check a decision runs, the author check included, before
// anything is tried, recorded or sent. A message already gone answers so.
func (s Service) Admit(ctx context.Context, reviewID ids.UUID, in consent.DirectInput) (Admitted, error) {
	intent, err := s.Consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return Admitted{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	if intent.IsZero() {
		return Admitted{}, consent.ErrNothingHeld
	}
	read, err := s.Consent.ReviewForReader(ctx, reviewID)
	if err != nil {
		return Admitted{}, err
	}
	if in, err = s.Consent.CheckDirection(ctx, reviewID, in); err != nil {
		return Admitted{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	return Admitted{Intent: intent, Read: read, In: in}, nil
}

// RefuseIfChanged answers a fresh review when the engine's refusals moved
// since the sender read theirs, so they decide again on what is true now.
// It answers nil when the read review still holds.
func (s Service) RefuseIfChanged(ctx context.Context, a Admitted, set commsauthz.DecisionSet, cause error) error {
	if !set.ExceptionMayAnswer(false) || a.Read.MatchesAssessment(set) {
		return nil
	}
	refreshed, err := s.Gate.RefreshRefusal(ctx, set, a.Intent)
	if err != nil {
		return err
	}
	return s.Gate.Refused(ctx, refreshed, set, cause, s.Actions(ctx, refreshed))
}

// Serve answers a Send anyway that names a moment, and reports false when the
// request names none, so the caller sends now.
func (s Service) Serve(
	w http.ResponseWriter, r *http.Request, reviewID ids.UUID, in consent.DirectInput,
	body crmcontracts.DirectCommunicationSendRequest,
) bool {
	sched, err := activities.ParseSendSchedule(body.ScheduledAt, body.ScheduledTz, body.ScheduledIntent)
	if err == nil && sched == nil {
		return false
	}
	var scheduled crmcontracts.ScheduledSend
	if err == nil {
		scheduled, err = s.DecideAndSchedule(r.Context(), reviewID, in, *sched)
	}
	sendmetrics.ObserveSendAnywayLater(err)
	if err != nil {
		httperr.Write(w, r, err)
		return true
	}
	httperr.WriteJSON(w, http.StatusAccepted, scheduled)
	return true
}

// DecideAndSchedule runs the checks Send anyway runs now, then asks the engine
// about the held message without sending it. Refusals that moved answer a
// fresh review at once. With none left, the message moves and no decision is
// recorded. Otherwise the decision and the move commit together.
func (s Service) DecideAndSchedule(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput, sched activities.SendSchedule,
) (crmcontracts.ScheduledSend, error) {
	if _, err := s.Sends.CheckLater(sched); err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	admitted, err := s.Admit(ctx, reviewID, in)
	if err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	// Another tab may send the message after Admit, so its sender hears that.
	move, err := s.Sends.PlanHeldMove(ctx, admitted.Intent, sched, s.Timer)
	if err != nil {
		return crmcontracts.ScheduledSend{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	set, err := s.assess(ctx, admitted.Intent)
	if err != nil {
		return crmcontracts.ScheduledSend{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	refused := len(consent.BlockingRefusals(set)) > 0
	if refused {
		if err := s.RefuseIfChanged(ctx, admitted, set, apperrors.ErrConsentNotGranted); err != nil {
			return crmcontracts.ScheduledSend{}, err
		}
	}
	err = s.DB.Tx(ctx, func(tx pgx.Tx) error {
		// The message row before the review, the order cancel and the fire take.
		if err := s.Sends.LockHeldTx(ctx, tx, move); err != nil {
			return err
		}
		if refused {
			if _, err := consent.DirectSendTx(ctx, tx, reviewID, admitted.In); err != nil &&
				!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
				return err
			}
		}
		return s.Sends.MoveHeldTx(ctx, tx, move)
	})
	if err != nil {
		return crmcontracts.ScheduledSend{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	return s.Sends.ScheduledSendWire(ctx, admitted.Intent)
}

// assess asks the engine about the held message as resuming it would, and
// records nothing.
func (s Service) assess(ctx context.Context, intent ids.UUID) (commsauthz.DecisionSet, error) {
	prepared, err := s.Sends.PrepareHeldSend(ctx, intent, s.Gate, s.Delivery)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	return s.Gate.Preview(ctx, prepared.Authorization())
}
