// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sendmetrics counts what a human's Send in the email composer came to,
// and how long the engine took to decide it. It also counts what each Send
// anyway confirmation came to. The pilot of the inline review reads its first-click success,
// decision latency and warning frequency from these families.
//
// Every label value is a constant in this file, chosen from a closed set. The
// observing functions take the send's outcome and error, never a label string,
// so no address, body, reason, user or workspace can become a label.
package sendmetrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
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

// The composer a Send came from, read off the inline review switch.
const (
	flowInline = "inline"
	flowLegacy = "legacy"
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
	confirmStale        = "stale_revision"
	confirmWrongWarning = "wrong_warning"
	confirmRefused      = "refused"
)

var (
	flows    = []string{flowInline, flowLegacy}
	outcomes = []string{outcomeQueued, outcomeScheduled, outcomeReview, outcomeRepair}
	contexts = []string{contextRoutine, contextOutreach}
	verdicts = []string{verdictAllowed, verdictRefused}
	confirms = []string{confirmSent, confirmStale, confirmWrongWarning, confirmRefused}
)

// decisionBounds are the decision histogram's upper bounds in seconds. The
// pilot's target is a p95 under one second, so the buckets are dense below it.
var decisionBounds = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5}

// pair keys every counter family: the flow and one label of its own.
type pair struct{ flow, value string }

type collector struct {
	mu        sync.Mutex
	outcomes  map[pair]uint64
	reviews   map[pair]uint64
	anyway    map[pair]uint64
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

func zeroed(values []string) map[pair]uint64 {
	out := map[pair]uint64{}
	for _, f := range flows {
		for _, v := range values {
			out[pair{flow: f, value: v}] = 0
		}
	}
	return out
}

// shared is the one collector this process counts into and renders.
var shared = newCollector()

// Observer is the activities send observer that counts into this process.
// inline reads the pilot switch for the caller.
func Observer(inline func(context.Context) (bool, error)) activities.SendObserver {
	return func(ctx context.Context, out activities.SendOutcome, err error) {
		shared.observeSend(ctx, inline, out, err)
	}
}

// observeSend counts one distinct first Send intent of a human. It skips an
// agent's send and a repeat press of a message already held. It also skips a
// failure that is no outcome, such as a blank subject.
func (c *collector) observeSend(ctx context.Context, inline func(context.Context) (bool, error), out activities.SendOutcome, err error) {
	if !humanAtTheKeyboard(ctx) || activities.IsRepeatedSend(err) {
		return
	}
	outcome, counted := outcomeOf(out, err)
	if !counted {
		return
	}
	on, readErr := inline(ctx)
	if readErr != nil {
		slog.WarnContext(ctx, "send metrics: the inline review switch could not be read, so this send is not counted", "err", readErr)
		return
	}
	flow := flowOf(on)
	refused, reviewed := errors.AsType[*consent.SendRefusedError](err)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcomes[pair{flow: flow, value: outcome}]++
	if reviewed && !refused.ReviewID.IsZero() {
		c.reviews[pair{flow: flow, value: contextOf(refused.Refusals)}]++
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

func flowOf(inline bool) string {
	if inline {
		return flowInline
	}
	return flowLegacy
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

// ObserveSendAnyway records what one Send anyway confirmation came to. The
// warning version it confirmed tells the inline band from the legacy dialog. A
// server fault is no answer about the confirmation, and is not counted.
func ObserveSendAnyway(warningVersion string, err error) {
	shared.observeSendAnyway(warningVersion, err)
}

func (c *collector) observeSendAnyway(warningVersion string, err error) {
	result, counted := confirmationOf(err)
	if !counted {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anyway[pair{flow: flowOf(warningVersion == consent.InlineWarningVersion), value: result}]++
}

func confirmationOf(err error) (string, bool) {
	if err == nil {
		return confirmSent, true
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
