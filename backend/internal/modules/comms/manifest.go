// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Checking a composed message against what its jurisdiction requires, at the
// last point it exists as it will be sent.
//
// THE LAST POINT IS THE ONLY HONEST ONE. Every obligation the packs declare is
// rendered somewhere earlier — a disclosure in the body, an unsubscribe header,
// a subject prefix — and each of those steps can be skipped, short-circuited or
// simply never reached by a path that composes its own message. A check here
// asks the question of the bytes about to leave rather than of the code that
// was supposed to produce them.
//
// A MISSING REQUIREMENT IS A FINDING, not a silent omission and not a refusal.
// gates/messagingruleapplied_test.go has carried "nothing prepends it" about
// the Vietnamese [QC] label since the pack shipped: an advertising message left
// unmarked whatever the decree said, and nothing anywhere reported it. That is
// what this closes.

import (
	"context"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// RequirementChecker answers what a message of this category still owes.
//
// Declared HERE by the consumer and implemented by consent, injected in
// compose/ like every other cross-module edge: comms hands a message to a
// provider and must not know what a jurisdiction pack is.
//
// A nil checker finds nothing, which is what every send did before this
// existed. The dozen test stores and the channel seam keep working without
// being taught about compliance packs.
type RequirementChecker interface {
	// CheckMessage answers one finding per requirement this message does not
	// meet. An empty answer means it meets them all.
	CheckMessage(ctx context.Context, m FinalMessage) ([]RequirementFinding, error)
}

// FinalMessage is the message as the provider will receive it.
//
// The fields a requirement can be about, and no more: a checker that could read
// the recipient list would be a jurisdiction pack reading addresses.
type FinalMessage struct {
	// DeliveryID is which send this is, so a finding can name it.
	DeliveryID string
	// DecisionSetID names the ONE set of decisions this send was authorized
	// on, which is what the requirement is judged against.
	//
	// Without it the checker would read the delivery's decisions by id alone,
	// and a delivery can hold several sets: a paced message redispatched after
	// its circumstances changed writes a fresh set beside the old one. Picking
	// among them would judge the message by a category it no longer has.
	DecisionSetID string
	// Subject is the line as composed, which is where a pack's prefix belongs.
	Subject string
	// Body and HTMLBody are the two alternatives. A requirement met in one and
	// not the other is met for whichever readers happen to fall back, which is
	// not a standard anybody writes down.
	Body     string
	HTMLBody string
	// ListUnsubscribe is the header, empty when the message carries none.
	ListUnsubscribe string
}

// RequirementFinding is one requirement a message does not meet.
type RequirementFinding struct {
	// Requirement names what is missing, in the vocabulary the packs use.
	Requirement string
	// Detail says what was expected, for whoever reads the record.
	Detail string
}

// String renders a finding for a log line or an error.
func (f RequirementFinding) String() string {
	if f.Detail == "" {
		return f.Requirement
	}
	return f.Requirement + ": " + f.Detail
}

// WithRequirementChecker wires what a jurisdiction demands of a composed
// message.
//
// An OPTION for the reason the controller relay is one: an installation that
// declares no country has no requirements to check, and a deployment should not
// have to name a nil to say so. Every test store that has no opinion about
// compliance packs keeps working unchanged.
func (d *Dispatcher) WithRequirementChecker(c RequirementChecker) *Dispatcher {
	d.requirements = c
	return d
}

// requirementsOf asks the checker, and answers nothing when none is wired.
func (d *Dispatcher) requirementsOf(
	ctx context.Context, del Delivery, setID string,
) ([]RequirementFinding, error) {
	if d.requirements == nil {
		return nil, nil
	}
	return d.requirements.CheckMessage(ctx, FinalMessage{
		DeliveryID:      del.ID.String(),
		DecisionSetID:   setID,
		Subject:         del.Subject,
		Body:            del.Body,
		HTMLBody:        del.HTMLBody,
		ListUnsubscribe: del.ListUnsubscribe,
	})
}

// describeFindings renders findings for the record, in a stable order.
func describeFindings(findings []RequirementFinding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, f.String())
	}
	return strings.Join(parts, "; ")
}

// gateRequirements refuses a message that does not meet what its jurisdiction
// requires.
//
// PARKED, NOT RETRIED. A missing subject prefix or an absent opt-out header is
// a property of the composed message, and the next attempt composes the same
// bytes from the same row: retrying would burn the ladder and park anyway,
// having sent nothing and said nothing useful in between. The park message
// names the requirement, which is the finding this gate exists to produce.
//
// NOT A REFUSAL OF THE SUBJECT'S RIGHTS, which is why it is not a consent
// verdict. Consent answers whether we may write to this contact; this answers
// whether what we composed is lawful to send at all. A message failing here
// would be unlawful to send to anybody.
//
// IT RUNS LAST of the refusals, on the bytes about to leave, because every
// earlier step is one that was supposed to put something in them — and the
// point is to ask the message rather than the code that should have produced
// it.
//
// AN ERROR HERE IS NOT A PASS, and the caller checks it alongside the outcome.
// Failing to ASK — an unreachable settings table, packs that cannot be resolved
// — leaves this undecided, and sending on that would be sending on a check
// nobody performed. That is the one direction this gate must never default to.
func (d *Dispatcher) gateRequirements(
	ctx context.Context, del Delivery, ticket commsauthz.TransmitTicket,
) (Outcome, time.Duration, error) {
	// MAIL ONLY, and this is not a convenience. Every requirement the packs
	// declare today is about a mail message's parts — a subject line, a footer,
	// an unsubscribe header. A channel delivery has no subject at all: staging
	// writes NULL and the provider's message carries no such field, so asking
	// the check about one would hand it an empty string, find the label
	// missing, and park every channel message on an obligation it has no way
	// to meet and was never under.
	//
	// A pack that one day declares a channel obligation needs a channel-shaped
	// FinalMessage and its own arm here, which is a change somebody makes
	// deliberately rather than one this gate makes by accident.
	if del.IsChannel() {
		return outcomeUndecided, 0, nil
	}
	findings, err := d.requirementsOf(ctx, del, ticket.DecisionSetID.String())
	if err != nil {
		return outcomeUndecided, 0, err
	}
	if len(findings) == 0 {
		return outcomeUndecided, 0, nil
	}
	return d.park(ctx, del.ID, "this message does not carry what its jurisdiction requires ("+
		describeFindings(findings)+")")
}
