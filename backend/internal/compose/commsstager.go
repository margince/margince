// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Staging one outbound message: the delivery row, the decision recording why it
// was allowed to be queued, and the job that will send it — all on the caller's
// transaction.
//
// Its own file because it is the seam where three modules meet and none may
// import another: activities hands down the message, comms owns the delivery
// row, consent owns the decision. Compose is the only place that may see all
// three, which is exactly why the wiring belongs here rather than in any of
// them.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// commsStager records an accepted send for transmission: the delivery row and
// the job that will carry it, both on the caller's transaction. One commit, one
// fact — a crash between them would either promise a send nothing queued or
// queue one with no timeline entry behind it.
type commsStager struct {
	store  *comms.Store
	runner *jobs.Runner
	// authority records why this message was allowed to be queued, on the same
	// transaction that queues it. Held here because compose is the only place
	// that may see both modules: comms owns the delivery row, consent owns the
	// decision, and neither may import the other.
	authority *consent.Gate
}

var (
	_ activities.DeliveryStager        = commsStager{}
	_ activities.ChannelDeliveryStager = commsStager{}
)

// DeliveryMachinery is the ONE delivery path in both the shapes a message can be
// staged in. It is a single seam rather than two because there is a single
// machinery behind it — one delivery table, one status machine, one retry ladder,
// one dispatcher — and a role able to wire mail staging without channel staging
// could serve a reply surface that accepts a message nothing will ever carry.
type DeliveryMachinery interface {
	activities.DeliveryStager
	activities.ChannelDeliveryStager
}

// NewDeliveryStager builds the delivery machinery every send transport is
// composed with (compose.WithDelivery). The runner is insert-only in the api
// role; the worker role works what it inserts.
//
//nolint:ireturn // returns the DeliveryMachinery seam by design: the concrete type is unexported and every caller holds the interface
func NewDeliveryStager(pool *pgxpool.Pool, runner *jobs.Runner) DeliveryMachinery {
	return commsStager{
		store:     comms.NewStore(InstallationDB(pool), time.Now, activities.NewStore(InstallationDB(pool))),
		runner:    runner,
		authority: consentGateFor(pool),
	}
}

func (s commsStager) StageTx(ctx context.Context, tx pgx.Tx, in activities.DeliveryRequest) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("comms: staging a delivery outside workspace context")
	}
	id, err := s.store.StageTx(ctx, tx, comms.StageInput{
		ActivityID:      in.ActivityID,
		Provider:        in.Provider,
		MessageID:       in.MessageID,
		Recipients:      in.Recipients,
		Cc:              in.Cc,
		Bcc:             in.Bcc,
		Subject:         in.Subject,
		Body:            in.Body,
		HTMLBody:        in.HTMLBody,
		FromName:        in.FromName,
		Attachments:     commsFiles(in.Attachments),
		ConsentPurpose:  in.ConsentPurpose,
		InReplyTo:       in.InReplyTo,
		References:      in.References,
		ThreadKey:       in.ThreadKey,
		ListUnsubscribe: in.ListUnsubscribe,
	})
	if err != nil {
		return err
	}
	// Why this was allowed to be queued, written before the job that will send
	// it. In the same transaction as the activity, the delivery row, the audit
	// entry and the outbox event: all of them or none, so a decision can never
	// describe a message that rolled back, and a delivery can never reach the
	// worker with nothing on record about why it exists.
	//
	// Not guarded on the authority being wired. A nil check here would read as
	// caution and behave as a bypass: a refactor that dropped the field would
	// compile, stage every message with no decision, and satisfy the census
	// gate — which sees the CALL and cannot see that it was skipped. The
	// transmit gate refuses a missing consent authority outright
	// (comms/gates.go), and this fails the same way.
	if s.authority == nil {
		return errors.New("compose: no authorization authority is wired on this send path")
	}
	// TOLD WHERE TO LOOK when this message is being resumed from a held row:
	// the intent names a review, and a review may carry a named human's
	// recorded decision that this refused message goes anyway. The decision is
	// resolved and spent inside this transaction, BEFORE the decision rows are
	// written, because the authority is part of what each row records and
	// those rows are never updated afterwards (migration 1788529047).
	set, instruction, err := s.authority.AuthorizeStagingWithDecisionTx(
		ctx, tx, id, in.Authorization, in.ResumingIntentID,
		consent.SendingDigest(in.AuthoredSubject, in.AuthoredBody, in.AuthoredHTML))
	if err != nil {
		return err
	}
	if refusal := refuseAtStaging(set); refusal != nil {
		// A REFUSAL IS NOT ALWAYS THE END. If this message is being resumed
		// from a held row, a named human may have read this very refusal and
		// decided in writing that it goes anyway. That decision is spent here,
		// inside the transaction that stages the message, so the send and the
		// spending are one fact.
		//
		// The refusal itself is untouched whichever way this goes: the decision
		// rows above still record what the engine said, and no suppression is
		// lifted. What a spent instruction changes is the AUTHORITY the message
		// leaves under.
		//
		// Already resolved and spent by the staging call above, which had to do
		// it there: the authority is written into the decision rows as they are
		// inserted, and re-asking here would spend a second decision after the
		// record of the first had already been written without it.
		if instruction.IsZero() {
			// CARRIED OUT, NOT WRITTEN HERE. Recording the review needs its own
			// transaction — this one is about to roll back and would take the
			// row with it — and opening one now would hold two connections from
			// the same pool at once. Sixteen concurrent refusals would then wait
			// on each other for a connection none of them can release.
			//
			// So the snapshot travels on the error, and whoever unwinds the
			// transaction writes it once the connection is back.
			return &pendingReviewError{set: set, cause: refusal}
		}
		// The delivery says so too, because the WORKER reads this row and never
		// reads the per-recipient decisions. A build that does not recognise
		// the authority parks the message rather than sending one it has no
		// rules for.
		if err := s.store.RecordDirectedExecutionTx(ctx, tx, id, instruction); err != nil {
			return err
		}
	}
	return s.runner.EnqueueTx(ctx, tx, SendEmailArgs{
		Workspace: ws, DeliveryID: id.String(),
	}, sendInsertOpts())
}

// pendingReviewError is a refusal travelling out of the transaction it stopped,
// carrying what the engine decided, so a review can be recorded after that
// transaction has unwound and given its connection back.
//
// It exists only between the staging call and RecordPendingReview. Nothing
// outside this file should see one: the recorder replaces it with the refusal
// the caller expects, wrapped with the review's reference.
type pendingReviewError struct {
	set   commsauthz.DecisionSet
	cause error
}

func (e *pendingReviewError) Error() string { return e.cause.Error() }

// Unwrap keeps the refusal's identity reachable, so a caller that never runs
// the recorder — a path this wiring has not reached yet — still answers the
// same 409 it always did rather than an unrecognised error.
func (e *pendingReviewError) Unwrap() error { return e.cause }

// RecordPendingReview turns a carried refusal into a durable review, AFTER the
// transaction that refused has finished unwinding.
//
// Called on the way out rather than at the point of refusal, because the
// staging call runs inside the caller's transaction and writing there would
// either be rolled back with it or hold a second pool connection while the
// first is still checked out.
//
// A FAILURE TO RECORD DOES NOT REPLACE THE REFUSAL. The send is refused either
// way, and answering a storage fault instead would tell the rep their message
// was fine and the database was not. The original refusal is what they need to
// see; the missing review is an operator's problem, not theirs.
// intentID names the held scheduled_send the sender froze this message into, so
// the review binds to something a human can resume. It is zero when nothing was
// held — a channel reply, or a hold that failed — and the review is then opened
// without an intent, which is every review this module wrote before the holder
// existed.
func (s commsStager) RecordPendingReview(ctx context.Context, err error, intentID ids.UUID) error {
	var pending *pendingReviewError
	if !errors.As(err, &pending) {
		return err
	}
	if s.authority == nil {
		return pending.cause
	}
	review, recordErr := s.authority.RecordRefusal(ctx, pending.set, intentID)
	if recordErr != nil || review.ID.IsZero() {
		return pending.cause
	}
	return &consent.SendRefusedError{ReviewID: review.ID, Cause: pending.cause}
}

// StageChannelTx is the same staging for a channel reply: the channel-shaped row
// and the SAME transmit job, on the caller's transaction.
//
// One job kind carries both shapes deliberately. The worker loads the delivery
// and dispatches it, and the dispatcher branches on the ROW's shape exactly once
// (comms/sendseam.go) — a second job kind would be a second path to keep in step
// with the first, and the channel is the one that would fall behind.
func (s commsStager) StageChannelTx(ctx context.Context, tx pgx.Tx, in activities.ChannelDeliveryRequest) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("comms: staging a channel delivery outside workspace context")
	}
	id, err := s.store.StageChannelTx(ctx, tx, comms.StageChannelInput{
		ActivityID:     in.ActivityID,
		Provider:       in.Provider,
		Recipient:      in.Recipient,
		Body:           in.Body,
		Attachments:    commsFiles(in.Attachments),
		ConsentPurpose: in.ConsentPurpose,
	})
	if err != nil {
		return err
	}
	// The channel path is a second implementation of staging, and this is the
	// half a fix to the mail path does not reach. It records the same decision
	// and fails closed the same way.
	if s.authority == nil {
		return errors.New("compose: no authorization authority is wired on this send path")
	}
	set, err := s.authority.AuthorizeStagingTx(ctx, tx, id, in.Authorization)
	if err != nil {
		return err
	}
	if err := refuseAtStaging(set); err != nil {
		// Carried out for the email path's reason: this transaction is about
		// to roll back, and opening a second one now would hold two pool
		// connections at once.
		return &pendingReviewError{set: set, cause: err}
	}
	return s.runner.EnqueueTx(ctx, tx, SendEmailArgs{
		Workspace: ws, DeliveryID: id.String(),
	}, sendInsertOpts())
}

// refuseAtStaging stops a send while the rep is still at the keyboard, for the
// two refusals that will not change between now and dispatch.
//
// The FIRST is an absolute denial — an Art. 21 objection, a processing
// restriction, a hard bounce, marketing whose round trip never completed — which
// no rollout mode may soften.
//
// The SECOND is a refusal under a category this installation ENFORCES. That arm
// exists because the engine now decides those: the transmit phase would refuse
// the same send anyway, so letting it stage buys nothing and costs two things.
// The rep learns minutes or days later, from a parked row in an operator lane
// rather than at the moment they pressed send. And the activity commits first,
// carrying the outbound attestation that makes an address
// correspondence-positive — so a message to somebody who may not receive it
// would mint evidence of correspondence that never happened.
//
// A category still OBSERVING is let through, and that is not an oversight: the
// engine's answer carries no authority there, the old gate decides, and blocking
// on a difference nobody has reviewed would refuse legitimate mail. The mode
// travels on each decision, so this asks the row rather than re-reading the
// setting the engine has already applied.
func refuseAtStaging(set commsauthz.DecisionSet) error {
	denied := set.Denied()
	if len(denied) == 0 {
		return nil
	}
	if !set.HasAbsoluteDenial() && !anyEnforcedDenial(denied) {
		return nil
	}
	return fmt.Errorf("consent: %d of %d recipients may not be written to (%s): %w",
		len(denied), len(set.Decisions), denied[0].ReasonCode, apperrors.ErrConsentNotGranted)
}

// anyEnforcedDenial reports whether a refusal was taken under a category this
// installation enforces, which is the one the engine's answer binds.
func anyEnforcedDenial(denied []commsauthz.Decision) bool {
	for _, d := range denied {
		if d.Mode == commsauthz.ModeEnforce {
			return true
		}
	}
	return false
}
