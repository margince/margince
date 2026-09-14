// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Handing the message to the provider, and reading what came back.
//
// Split from dispatcher.go, which decides WHETHER a delivery goes: the gates,
// the pacing and the ladder. This is what happens once that is settled, and the
// two halves change for different reasons — a new refusal is a gate, a new
// provider failure mode is here.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// transmit hands the message to the provider and records what came back. The
// seam already carries the shape-specific half (sendseam.go), so what follows is
// the same for a mail message and a channel one.
func (d *Dispatcher) transmit(ctx context.Context, del Delivery, seam sendSeam, ticket commsauthz.TransmitTicket) (Outcome, time.Duration, error) {
	// The last thing checked before the wire, and the reason the ticket is
	// threaded down here rather than trusted from three frames up: a send that
	// reaches a provider without a decision recorded for THIS delivery and THIS
	// attempt is a send nobody can account for afterwards. A stale attempt is
	// as bad as none — it belongs to a try whose world may already have moved.
	if !ticket.Current(del.ID, del.Attempts) {
		return d.park(ctx, del.ID, "no current authorization decision covers this attempt")
	}
	// The attachment BYTES, resolved before the marker below and not inside the
	// provider call.
	//
	// The ordering is the whole point. A seam that cannot detect a prior send
	// commits an in-flight marker and then treats any later attempt as "the
	// outcome was never learned", so a fault BETWEEN the marker and the wire —
	// an unreadable object, a store that would not answer, a file set too large
	// to carry — would park a message that never left with a reason telling the
	// rep it may have arrived and discouraging a resend. Read first, and such a
	// fault is what it is: nothing transmitted, and the ladder may try again.
	//
	// They are still not on the delivery row: a message on a retry ladder would
	// otherwise hold every attachment it might ever send in the database,
	// duplicated per delivery, for as long as the maximum age allows.
	files, err := d.attachedFiles(ctx, del)
	if err != nil {
		return d.retry(ctx, del.ID, fmt.Errorf("comms: reading this message's files before transmitting: %w", err))
	}
	// At-most-once, for the seams that need it: a transmission whose outcome was
	// never learned is never attempted a second time.
	if outcome, wait, err := d.guardAtMostOnce(ctx, del, seam); outcome != outcomeUndecided {
		return outcome, wait, err
	}
	receipt, err := seam.transmit(ctx, files)
	if err != nil {
		return d.classifySendFailure(ctx, del, err)
	}

	if err := d.store.RecordSent(ctx, del.ID, receipt); err != nil {
		if errors.Is(err, ErrTerminal) {
			// A newer attempt already closed this row against its own
			// receipt; overwriting it would replace a real one.
			return OutcomeSkipped, 0, nil
		}
		if !seam.detectsPriorSend {
			return d.parkTransmitted(ctx, del, receipt, err)
		}
		// Mail goes back on the ladder: the next attempt's prior-send lookup
		// finds the message at the provider and answers from it rather than
		// transmitting a second copy, so the receipt is recorded late instead of
		// lost.
		return OutcomeRetry, 0, fmt.Errorf("comms: recording the send receipt: %w", err)
	}

	// The one-time link has now been sent and must stop being live.
	d.retireLink(ctx, del)

	// Metering follows the DURABLE record, not the provider call, because the
	// send call is not the countable event: a receipt that failed to record
	// comes back on the ladder, and Send answers a retry from Gmail's
	// prior-send lookup rather than transmitting again. Metering at the call
	// would count that one message twice. RecordSent is guarded on
	// status = 'pending' and reports ErrTerminal otherwise, so exactly one
	// attempt per delivery ever reaches this line — which is what makes
	// "metered" mean "one message, once".
	//
	// Policies are told here rather than at Wait for the same reason in the
	// other direction: a limiter counting checks instead of sends paces
	// nothing.
	//
	// Metered only for a sender the policies actually pace. A controller row is
	// not paced, and its user id is the zero value, so recording it would fill
	// one bucket forever under a key naming nobody — harmless until somebody
	// turns pacing on for this sender and finds it already throttled by years
	// of accumulated slots.
	if profileFor(del).paced {
		for _, policy := range d.policies {
			if recorder, meters := policy.(SendRecorder); meters {
				recorder.Recorded(del)
			}
		}
	}
	return OutcomeSent, 0, nil
}

// receiptUnrecordedReason is what a delivery the provider ACCEPTED records when
// its receipt could not be written. It is the opposite fact to
// unknownOutcomeReason and must never be confused with it: nothing here is
// uncertain — the message went — so the sentence tells the operator the one
// thing they must not do about it.
const receiptUnrecordedReason = "the provider accepted this message and its receipt could not be recorded: " +
	"it WAS sent, and the provider's own message id is kept on this delivery. " +
	"Do not send it again — the recipient already has it"

// parkTransmitted closes a delivery whose message is already with the provider
// and whose receipt this attempt could not write.
//
// It exists for the seams whose retries cannot detect a prior send. For those,
// returning the attempt to the ladder is not a delay but a LOSS: the next
// attempt reads the in-flight marker, learns nothing about what happened, and
// parks the delivery as an outcome nobody knows — a message the customer is
// holding, durably recorded as never sent. Parking here instead states what is
// definitely true and keeps the provider's message id, which after a failed
// receipt is the only handle left on that message.
func (d *Dispatcher) parkTransmitted(ctx context.Context, del Delivery, receipt connector.SendReceipt, cause error) (Outcome, time.Duration, error) {
	if err := d.store.ParkTransmitted(ctx, del.ID, receiptUnrecordedReason, receipt.ProviderMessageID); err != nil {
		if errors.Is(err, ErrTerminal) {
			return OutcomeSkipped, 0, nil
		}
		// Nothing durable records this send now. The row stays pending with its
		// marker standing, so the next attempt parks on the uncertainty rather
		// than messaging the customer twice, and both causes reach the job log.
		return OutcomeRetry, 0, errors.Join(cause, err)
	}
	// The cause goes to the LOG rather than back to the caller. The delivery is
	// terminal — a returned error would fail a job whose work is done and put a
	// closed row back on the ladder — and the reason column may not carry a
	// database fault's own text (faultReason), so this is where an operator's
	// diagnosis lives.
	slog.ErrorContext(ctx, "comms: a transmitted message's receipt could not be recorded; the delivery is parked against it",
		"err", cause, "delivery_id", del.ID, "provider_message_id", receipt.ProviderMessageID)
	return OutcomeParked, 0, nil
}

// classifySendFailure turns a provider failure into a disposition using only
// the shared sentinel vocabulary, so the provider's own text stops at the
// connector boundary.
//
// A permanent rejection is recognized only where the SEAM can prove it:
// ErrRecipientUnreachable is reported by a provider that answers a refused
// recipient differently from a refused credential, and it parks at once. The
// Gmail connector cannot — it maps every non-throttled, non-2xx response to
// ErrUnreachable, so a refused mail recipient is indistinguishable from an
// outage there and still burns the whole retry ladder before its job exhausts
// and the delivery parks.
func (d *Dispatcher) classifySendFailure(ctx context.Context, del Delivery, err error) (Outcome, time.Duration, error) {
	if errors.Is(err, connector.ErrSendOutcomeUnknown) {
		// NEVER retried, and no shape test is needed to decide that: only a seam
		// that cannot discover a prior send reports this class, and one that can
		// is obliged to go and find out instead. The in-flight marker
		// deliberately STAYS — it is the durable record that a message may
		// already be with the customer, and the park reason is the only honest
		// thing to tell the operator reading the row.
		return d.park(ctx, del.ID, unknownOutcomeReason)
	}
	// Everything below is a DEFINITE answer from the provider, which proves
	// nothing was transmitted — so the in-flight marker is retracted before the
	// delivery goes back on the ladder. It is a no-op for a seam that never set
	// one, which is what keeps this a single rule rather than a second branch on
	// provider class.
	if clearErr := d.store.ClearInFlight(ctx, del.ID); clearErr != nil && !errors.Is(clearErr, ErrTerminal) {
		// The marker is still standing, so the next attempt will park rather
		// than re-send. Both causes go back for the job log, and the delivery
		// errs toward an unsent message — the direction this whole path is built
		// to err in.
		return d.retry(ctx, del.ID, errors.Join(err, clearErr))
	}
	if errors.Is(err, connector.ErrAuthRejected) {
		return d.park(ctx, del.ID, "the provider rejected the credential this delivery transmits through; reconnect it to resume sending")
	}
	// Checked alongside the credential class, not after the ladder: the two are
	// the pair an operator most easily confuses, and the whole value of telling
	// them apart is that each row says which one it was.
	if errors.Is(err, connector.ErrRecipientUnreachable) {
		return d.park(ctx, del.ID, unreachableRecipientReason)
	}
	// The adapter refused the file set itself. This is a DECISION rather than a
	// provider condition, so it cannot come out differently on a later attempt:
	// left on the ladder it would re-read every file from the blobstore once per
	// rung and then park under "the retry ladder is exhausted", which names no
	// cause at all. The carriage gate catches this case earlier from the
	// capability a connector DECLARES; this is the connector refusing what its
	// own send path cannot honour, which is the half no gate above it can see.
	if errors.Is(err, connector.ErrFilesNotCarried) {
		// The cause is LOGGED rather than dropped. filesNotCarriedReason cannot
		// carry it — a park reason is read by the contact who wrote the message,
		// and the refusals below the gate name a file, a byte count and a bound
		// that were built for an operator — but those are the only statement of
		// WHICH file and WHICH limit ended this delivery. Parking returns nil, so
		// without this line the job succeeds and the sentence disappears.
		slog.ErrorContext(ctx, "comms: the channel refused the files this message was staged with; the delivery is parked",
			"delivery_id", del.ID, "provider", del.Provider, "err", err)
		return d.park(ctx, del.ID, filesNotCarriedReason)
	}
	// Honour the provider's own interval when it named one: it knows when it
	// will accept the next message, and guessing shorter earns another
	// throttle. A rate limit with no stated interval leaves nothing to
	// honour, so it falls through to the retry ladder rather than asking the
	// caller to re-run immediately against a provider already throttling us.
	if limited, throttled := errors.AsType[*connector.RateLimitedError](err); throttled && limited.RetryAfter > 0 {
		return d.throttled(ctx, del, limited.RetryAfter)
	}
	return d.retry(ctx, del.ID, err)
}
