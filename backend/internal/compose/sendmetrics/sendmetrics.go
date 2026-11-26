// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sendmetrics counts what a human's Send in the email composer came to,
// and how long the engine took to decide it. It also counts what each Send
// anyway confirmation came to. First-click success, decision latency and
// warning frequency are read from these families.
//
// Every label value is a constant in this file, chosen from a closed set. The
// observing functions take the send's outcome and error, never a label string,
// so no address, body, reason, user or workspace can become a label.
package sendmetrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// What a first Send came to. Review is a refusal its sender may send anyway;
// repair is a refusal they may not, such as a hard bounce.
const (
	outcomeQueued    = "queued"
	outcomeScheduled = "scheduled"
	outcomeReview    = "review"
	outcomeRepair    = "repair"
)

// The context class of a refused Send: outreach when the engine resolved any
// refused recipient to marketing, routine otherwise.
const (
	contextRoutine  = "routine"
	contextOutreach = "outreach"
)

const (
	verdictAllowed = "allowed"
	verdictRefused = "refused"
)

// What a Send anyway confirmation came to. Refused is any other refusal the
// caller was told about, such as the engine refusing the resumed message again.
const (
	confirmSent         = "sent"
	confirmScheduled    = "scheduled"
	confirmAlreadySent  = "already_sent"
	confirmStale        = "stale_revision"
	confirmWrongWarning = "wrong_warning"
	confirmRefused      = "refused"
)

var (
	outcomes = []string{outcomeQueued, outcomeScheduled, outcomeReview, outcomeRepair}
	contexts = []string{contextRoutine, contextOutreach}
	verdicts = []string{verdictAllowed, verdictRefused}
	confirms = []string{confirmSent, confirmScheduled, confirmAlreadySent, confirmStale, confirmWrongWarning, confirmRefused}
)

// decisionBounds are the decision histogram's upper bounds in seconds. The
// target is a p95 under one second, so the buckets are dense below it.
var decisionBounds = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5}

// Every counter family is keyed by its one label's value.
type collector struct {
	mu        sync.Mutex
	outcomes  map[string]uint64
	reviews   map[string]uint64
	anyway    map[string]uint64
	decisions map[string]*httpserver.Histogram
}

// newCollector starts every series of the closed label sets at zero, so a
// ratio over them has a denominator from the first scrape.
func newCollector() *collector {
	c := &collector{
		outcomes: zeroed(outcomes), reviews: zeroed(contexts), anyway: zeroed(confirms),
		decisions: map[string]*httpserver.Histogram{},
	}
	for _, v := range verdicts {
		c.decisions[v] = httpserver.NewHistogram(decisionBounds)
	}
	return c
}

func zeroed(values []string) map[string]uint64 {
	out := make(map[string]uint64, len(values))
	for _, v := range values {
		out[v] = 0
	}
	return out
}

// shared is the one collector this process counts into and renders.
var shared = newCollector()

// Observer is the activities send observer that counts into this process.
func Observer() activities.SendObserver {
	return shared.observeSend
}

// observeSend counts one distinct first Send intent of a human. It skips an
// agent's send and a repeat press of a message already held. It also skips a
// failure that is no outcome, such as a blank subject.
func (c *collector) observeSend(ctx context.Context, out activities.SendOutcome, err error) {
	if !humanAtTheKeyboard(ctx) || activities.IsRepeatedSend(err) {
		return
	}
	outcome, counted := outcomeOf(out, err)
	if !counted {
		return
	}
	refused, reviewed := errors.AsType[*consent.SendRefusedError](err)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcomes[outcome]++
	if reviewed && !refused.ReviewID.IsZero() {
		c.reviews[contextOf(refused.Refusals)]++
	}
}

func outcomeOf(out activities.SendOutcome, err error) (string, bool) {
	switch {
	case err == nil && out.Scheduled != nil:
		return outcomeScheduled, true
	case err == nil:
		return outcomeQueued, true
	case !errors.Is(err, apperrors.ErrConsentNotGranted):
		return "", false
	}
	if refused, ok := errors.AsType[*consent.SendRefusedError](err); ok && slices.Contains(refused.Actions, consent.ActionDirectSend) {
		return outcomeReview, true
	}
	return outcomeRepair, true
}

func contextOf(refusals []consent.RefusedRecipient) string {
	for _, r := range refusals {
		if commsauthz.Category(r.Category) == commsauthz.CategoryMarketing {
			return contextOutreach
		}
	}
	return contextRoutine
}

// ObserveDecision records how long the engine took to decide a human's fresh
// Send at staging. A resumed message (Send anyway, or a scheduled send firing)
// names the held row it resumes, and is not a fresh Send.
func ObserveDecision(ctx context.Context, resuming ids.UUID, elapsed time.Duration, allowed bool) {
	if !resuming.IsZero() || !humanAtTheKeyboard(ctx) {
		return
	}
	shared.observeDecision(allowed, elapsed)
}

func (c *collector) observeDecision(allowed bool, elapsed time.Duration) {
	verdict := verdictRefused
	if allowed {
		verdict = verdictAllowed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decisions[verdict].Observe(elapsed.Seconds())
}

// ObserveSendAnyway records what one Send anyway confirmation came to. A
// server fault is no answer about the confirmation, and is not counted.
func ObserveSendAnyway(err error) {
	shared.observeSendAnyway(err, confirmSent)
}

// ObserveSendAnywayLater records a Send anyway that named a moment, which
// schedules the message rather than sending it.
func ObserveSendAnywayLater(err error) {
	shared.observeSendAnyway(err, confirmScheduled)
}

func (c *collector) observeSendAnyway(err error, done string) {
	result, counted := confirmationOf(err, done)
	if !counted {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anyway[result]++
}

func confirmationOf(err error, done string) (string, bool) {
	if err == nil {
		return done, true
	}
	if _, sent := errors.AsType[*consent.MessageAlreadySentError](err); sent {
		return confirmAlreadySent, true
	}
	if _, stale := errors.AsType[*consent.InstructionStaleError](err); stale {
		return confirmStale, true
	}
	if consent.WrongWarningVersion(err) {
		return confirmWrongWarning, true
	}
	if fault, ok := httperr.Classify(err); ok && fault.Status < http.StatusInternalServerError {
		return confirmRefused, true
	}
	return "", false
}

func humanAtTheKeyboard(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	return ok && actor.Type == principal.PrincipalHuman
}

// WriteProcessMetrics renders this process's send families.
func WriteProcessMetrics(w io.Writer) { shared.writePrometheus(w) }
