// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sendlater is Send anyway with a moment: the sender's decision is
// recorded now, and the held message waits for the moment they chose. The
// fire at that moment asks the engine again and spends the decision.
package sendlater

import (
	"context"
	"errors"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/sendmetrics"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Service holds the two modules a later Send anyway spans.
type Service struct {
	Consent *consent.Store
	Sends   *activities.Store
	Timer   activities.ScheduleTimer
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

// DecideAndSchedule checks the moment first, so a bad one records nothing.
// Then the decision, which runs every check a Send anyway runs. Then the move.
// A move that fails leaves the decision standing on a held message, and a
// retry finds it already recorded.
func (s Service) DecideAndSchedule(
	ctx context.Context, reviewID ids.UUID, in consent.DirectInput, sched activities.SendSchedule,
) (crmcontracts.ScheduledSend, error) {
	sched, err := s.Sends.CheckLater(sched)
	if err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	intent, err := s.Consent.HeldMessageForReview(ctx, reviewID)
	if err != nil {
		return crmcontracts.ScheduledSend{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	if intent.IsZero() {
		return crmcontracts.ScheduledSend{}, consent.ErrNothingHeld
	}
	if _, err := s.Consent.DirectSend(ctx, reviewID, in); err != nil &&
		!errors.Is(err, consent.ErrDecisionAlreadyRecorded) {
		return crmcontracts.ScheduledSend{}, s.Consent.SentInstead(ctx, reviewID, err)
	}
	return s.Sends.ScheduleHeldLater(ctx, intent, sched, s.Timer)
}
