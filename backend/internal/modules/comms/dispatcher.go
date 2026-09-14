// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Outcome is what one dispatch attempt concluded. It is the caller's whole
// instruction: a job runner maps it to "done", "snooze", or "back off" without
// re-deriving anything from the delivery row.
type Outcome string

const (
	// OutcomeSent means the provider has the message and the receipt is recorded.
	OutcomeSent Outcome = "sent"
	// OutcomeSkipped means there was nothing left to do — the delivery was
	// already terminal when this attempt reached it.
	OutcomeSkipped Outcome = "skipped"
	// OutcomePostponed means the delivery may still go, but not yet; the
	// returned wait is how long the caller should defer it.
	OutcomePostponed Outcome = "postponed"
	// OutcomeParked means the delivery will never go and the row says why.
	OutcomeParked Outcome = "parked"
	// OutcomeRetry means the attempt failed to reach a verdict; the delivery
	// stays pending for the caller's retry ladder.
	OutcomeRetry Outcome = "retry"
)

// Dispatcher runs one delivery attempt: the fixed gates that can refuse it, the
// configurable policy chain that can postpone it, and the transmission itself.
//
// Gates and policies are deliberately different mechanisms because they are
// different facts. A gate says NEVER — no amount of waiting repairs a revoked
// grant or a withdrawn consent — so gates are inline, fixed, and not
// configurable. A policy says NOT YET, so policies are an ordered chain the
// deployment assembles.
type Dispatcher struct {
	store       deliveryStore
	resolver    ConnectionResolver
	seats       SeatAuthority
	attachments AttachmentAuthority
	consent     ConsentGate
	policies    []SendPolicy
	now         func() time.Time
	maxAge      time.Duration
	maxAttempts int
	// relay and payloads serve the controller lane only. Both nil on an
	// installation that sends no controller mail, which resolveSeam reports as
	// a parked delivery rather than a retry: an unconfigured relay is a
	// deployment fact and waiting does not change it.
	relay    ControllerRelay
	payloads PayloadVault
	// requirements checks the composed message against what its jurisdiction
	// demands. Nil on an installation that declares no country, and on every
	// test store that has no opinion about compliance packs.
	requirements RequirementChecker
}

// defaultMaxAttempts bounds a dispatcher whose ladder length was not
// configured. A missing bound must still park eventually: once the runner
// stops delivering an exhausted job nothing else moves the row off pending,
// and a row that looks live forever is the failure the exhaustion guard exists
// to prevent. Disabling the guard on a non-positive bound would trade a loud
// catastrophe for a silent one, so it defaults rather than disappearing.
//
// The value is a generous finite ceiling, not a claim about any particular
// runner's ladder — a caller that knows its own should pass it.
const defaultMaxAttempts = 25

// minMaxAttempts is the floor a configured ladder length is raised to. One
// rung is arithmetically positive and survives the default above, but Load
// counts an attempt BEFORE the exhaustion guard reads the counter, so a bound
// of one would meet `Attempts >= maxAttempts` on the very first dispatch and
// park every delivery without ever asking a provider. Two rungs is the
// smallest bound under which the guard bounds a ladder rather than replacing
// it.
const minMaxAttempts = 2

// NewDispatcher builds the dispatcher. maxAge bounds how long a delivery may
// be postponed before it parks instead, and maxAttempts is the caller's retry
// ladder length — the dispatcher parks on the last rung rather than leaving a
// row the runner will never deliver again looking pending forever.
//
// Neither knob can be set to the absence of the behaviour it configures: a nil
// clock DEFAULTS to time.Now, and maxAttempts defaults to defaultMaxAttempts
// when unset and is floored at minMaxAttempts when below it. A caller that
// forgets one gets the conservative version of the rule, never no rule.
func NewDispatcher(
	store deliveryStore,
	resolver ConnectionResolver,
	seats SeatAuthority,
	attachments AttachmentAuthority,
	consent ConsentGate,
	policies []SendPolicy,
	now func() time.Time,
	maxAge time.Duration,
	maxAttempts int,
) *Dispatcher {
	if now == nil {
		now = time.Now
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	if maxAttempts < minMaxAttempts {
		maxAttempts = minMaxAttempts
	}
	return &Dispatcher{
		store: store, resolver: resolver, seats: seats, attachments: attachments,
		consent: consent, policies: policies,
		now: now, maxAge: maxAge, maxAttempts: maxAttempts,
	}
}

// WithControllerRelay wires the transport for the installation's own mail, and
// the vault holding the one-time links it carries.
//
// An OPTION rather than a constructor parameter because the lane is optional:
// every other send needs a resolver, a seat authority and a consent gate, and a
// deployment that sends no confirmation mail should not have to name two nils
// to say so.
func (d *Dispatcher) WithControllerRelay(relay ControllerRelay, payloads PayloadVault) *Dispatcher {
	d.relay = relay
	d.payloads = payloads
	return d
}

// DispatchWithWait runs one delivery attempt and reports how long to wait when
// the outcome is OutcomePostponed (zero for every other outcome).
//
// The sequence is authority → consent → pacing, and the order is load-bearing
// rather than stylistic: authority must refuse BEFORE consent answers, or the
// difference between "you may not" and "they said no" tells a caller with no
// rights at all something about a contact's consent state.
func (d *Dispatcher) DispatchWithWait(ctx context.Context, id ids.UUID) (Outcome, time.Duration, error) {
	// Load counts this attempt and refuses a delivery that already finished.
	// Job delivery is at-least-once, and that terminal status — not any
	// in-flight claim, of which there is none by design — is what makes a
	// redelivery safe: a redelivered job stops here instead of mailing a
	// second copy.
	del, err := d.store.Load(ctx, id)
	if errors.Is(err, ErrTerminal) {
		return OutcomeSkipped, 0, nil
	}
	if err != nil {
		// A load that failed to answer is an outage, not a verdict, and
		// there is no row in hand to record a reason against.
		return OutcomeRetry, 0, err
	}

	if del.carriesLiveLink() {
		return d.dispatchClosingLink(ctx, del)
	}
	return d.dispatchLoaded(ctx, del)
}

// dispositionForUnresolvedSeam turns a failure to resolve the transmitting
// credential into the disposition it deserves.
//
// The split between park and retry is the whole point, and getting it backwards
// is expensive in both directions: parking on an outage destroys a legitimate
// send, and retrying an ANSWER — no mailbox connected, no relay configured —
// leaves a message rattling round the ladder while nobody learns the thing they
// have to fix. Every named case here is an answer; anything else is a failure to
// get one.
func (d *Dispatcher) dispositionForUnresolvedSeam(ctx context.Context, del Delivery, err error) (Outcome, time.Duration, error) {
	switch {
	case errors.Is(err, ErrNoMailbox):
		return d.park(ctx, del.ID, fmt.Sprintf(
			"nothing is connected for %s to transmit through; connect it to enable sending", del.Provider))
	case errors.Is(err, ErrCannotSend):
		return d.park(ctx, del.ID, fmt.Sprintf("the %s connection cannot transmit messages", del.Provider))
	case errors.Is(err, ErrNoControllerRelay):
		return d.park(ctx, del.ID,
			"this installation has no mail relay configured to send its own notices through; configure one, then re-send")
	case errors.Is(err, ErrProviderNotConfigured):
		return d.park(ctx, del.ID, fmt.Sprintf(
			"this installation has no %s integration configured to transmit through; configure it, then re-send", del.Provider))
	default:
		return d.retry(ctx, del.ID, err)
	}
}

// dispatchLoaded runs the gates and the transmit for a delivery already loaded.
func (d *Dispatcher) dispatchLoaded(ctx context.Context, del Delivery) (Outcome, time.Duration, error) {
	// Resolve first, because the authority gate reads the scopes the provider
	// says this grant holds right now — not a copy stored when it was granted.
	// resolveSeam is the ONE branch on provider class (sendseam.go); everything
	// from here down is one path for both transports.
	seam, err := d.resolveSeam(ctx, del)
	if err != nil {
		return d.dispositionForUnresolvedSeam(ctx, del, err)
	}

	// Which of the gates below apply at all. A controller message has no seat,
	// no mailbox grant and no attachments, so those questions are not softened
	// for it — they are not ITS questions. Authorization and the ladder bound
	// are asked of every sender.
	profile := profileFor(del)

	// Gate: authority. It refuses first so that a caller with no rights at
	// all learns nothing about the recipients' consent state.
	if profile.requiresSendScope {
		if outcome, wait, err := d.gateSendAuthority(ctx, del, seam.granted); outcome != outcomeUndecided {
			return outcome, wait, err
		}
	}

	// Gate: the sender's seat, which is authority-class and therefore belongs
	// here rather than after consent. The mailbox grant above is the
	// PROVIDER's answer about a credential; this is THIS installation's answer
	// about the human it was lent by, and deactivating them touches neither
	// the connection nor the grant.
	if profile.requiresLiveSeat {
		if outcome, wait, err := d.gateSeat(ctx, del); outcome != outcomeUndecided {
			return outcome, wait, err
		}
	}

	// Gate: the authority this delivery claims to go out under, BEFORE consent
	// is asked. The consent gate honours a recorded instruction, so it has to
	// know that this build understands what the row says — an unrecognised
	// value must park rather than reach a gate that might read it as
	// permission.
	if outcome, wait, err := d.gateExecutionAuthority(ctx, del); outcome != outcomeUndecided {
		return outcome, wait, err
	}

	// Gate: suppression and consent, which are one step — one-click
	// unsubscribe writes a per-purpose consent withdrawal, so this gate IS
	// the suppression mechanism.
	ticket, outcome, wait, err := d.gateConsent(ctx, del)
	if outcome != outcomeUndecided {
		return outcome, wait, err
	}

	// Gate: attachment carriage. It runs with the other refusals rather than at
	// the provider call, because the answer is known before any I/O and a
	// message that cannot go out intact should never reach the wire at all.
	if profile.carriesFiles {
		if outcome, wait, err := d.gateAttachmentCarriage(ctx, del, seam); outcome != outcomeUndecided {
			return outcome, wait, err
		}
	}

	// Gate: the files themselves, rechecked against now rather than against
	// staging time. It runs after carriage because carriage needs no I/O and
	// this needs a read, so the free refusal is asked first.
	if profile.carriesFiles {
		if outcome, wait, err := d.gateAttachmentIntegrity(ctx, del); outcome != outcomeUndecided {
			return outcome, wait, err
		}
	}

	// Gate: what the jurisdiction requires of the composed message, asked last
	// and of the bytes about to leave. gateRequirements says why, including why
	// its error is checked alongside its outcome.
	if outcome, wait, err := d.gateRequirements(ctx, del, ticket); outcome != outcomeUndecided || err != nil {
		return outcome, wait, err
	}

	// Policies postpone; they never refuse. They run after both gates, so a
	// delivery that may never go is refused rather than paced.
	if profile.paced {
		if outcome, wait, err := d.pace(ctx, del); outcome != outcomeUndecided {
			return outcome, wait, err
		}
	}

	// Ladder exhaustion. Once the runner stops delivering this job nothing
	// else would ever move the row off pending, and it would look live
	// forever. NewDispatcher floors the bound at minMaxAttempts, which is what
	// keeps this from parking a delivery on its first attempt — Load counts
	// the attempt before the comparison reads it.
	if del.Attempts >= d.maxAttempts {
		return d.park(ctx, del.ID, fmt.Sprintf("the retry ladder is exhausted after %d attempts", del.Attempts))
	}

	return d.transmit(ctx, del, seam, ticket)
}

// pace applies the policy chain. The chain is ordered and the first non-zero
// wait wins, so adding a policy is a registration rather than a change to the
// dispatch sequence. It returns outcomeUndecided when every policy permits the
// delivery to go now.
func (d *Dispatcher) pace(ctx context.Context, del Delivery) (Outcome, time.Duration, error) {
	for _, policy := range d.policies {
		wait := policy.Wait(ctx, del)
		if wait <= 0 {
			continue
		}
		// A permanently saturated policy would defer this delivery forever,
		// silently — which looks fine right up until someone's email never
		// went out. Past the maximum age it parks with a reason instead.
		if age := d.now().Sub(del.CreatedAt); age > d.maxAge {
			return d.park(ctx, del.ID, fmt.Sprintf(
				"policy %q deferred this delivery for %s, past the %s maximum age",
				policy.Name(), age.Round(time.Second), d.maxAge,
			))
		}
		return d.postpone(ctx, del.ID, "waiting: "+policy.Name(), wait)
	}
	return outcomeUndecided, 0, nil
}
