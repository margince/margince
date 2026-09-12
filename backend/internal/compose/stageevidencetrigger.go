// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Two records that already settle something, turned into stage evidence.
//
//   contract.status_changed → active            → document_signed
//   activity.captured / activity.updated,
//     a meeting that took place                 → event_held
//
// THE TRIGGER IS THE EVENT, NOT THE WRITER, as captureenrichtrigger states:
// each of these reaches the outbox because the write shape puts it there, so
// every path that signs a contract or captures a meeting lands here without
// knowing this consumer exists.
//
// terms_accepted has NO deterministic writer. The deal room is a place to
// discuss a document, not to accept one — nothing in it is an acceptance —
// and deal_room.decision_recorded, which an earlier version of this file
// consumed, is retired: public-events.yaml marks it HISTORICAL ONLY and
// nothing emits it. A criterion of that kind waits for the model's half of
// the ledger, which reads what the buyer actually wrote.
//
// No model is asked anything here. Each fact is one a record states outright.
//
// It writes DIRECTLY rather than queuing a pass. The capture trigger queues
// because its work is a model-backed read with a budget; this one's work is a
// short transaction against rows the event already names, and a job would add
// a queue hop to something that finishes in a millisecond.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageEvidenceBacklogWindow bounds how old a RECORD may be and still write
// evidence.
//
// The first-boot hazard captureenrichtrigger names: a new consumer group starts
// at stream position 0, so the first boot replays every contract and meeting
// this installation has ever recorded. Without a bound each would mint evidence
// against whatever stage its deal sits on now, which is a ledger describing a
// history that never happened.
//
// SEVEN DAYS, and measured against the event's own occurrence — not against
// how long it took to be delivered. An hour of delivery lag was the earlier
// bound, and it silently discarded real work: a worker down for ninety minutes
// dropped every signature in that window PERMANENTLY, because skipping answers
// nil, the subscriber acks, and nothing replays it. Delivery lag is an
// operational fact about this installation; how long ago the meeting happened
// is a fact about the record, and only the second one bears on whether the
// evidence is still true.
const stageEvidenceBacklogWindow = 7 * 24 * time.Hour

// stageEvidenceActor is what the audit trail names as the writer of these
// rows. No human asked for them, and binding the last contact to touch the deal
// would put their name on an observation they never made.
const stageEvidenceActor = "system:stage-evidence"

// The three event types this consumer acts on. Constants rather than string
// literals in the switch, so a typo is a compile error rather than a lane that
// comes up and silently never fires.
const (
	eventContractStatusChanged = "contract.status_changed"
	eventActivityCaptured      = "activity.captured"
	eventActivityUpdated       = "activity.updated"
)

// activityKindMeeting is the activity.kind a meeting carries. Read from the row
// rather than taken from an event payload, because activity.updated does not
// name the kind and a task must not be judged as a meeting.
const activityKindMeeting = "meeting"

// StageEvidenceReadEnqueuer queues the model's half of the ledger. Exported
// because cmd/worker decides whether this installation has a lane to read on,
// and a nil one is the legal composition that says it does not.
type StageEvidenceReadEnqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// StageEvidenceTrigger writes record-derived evidence as the records land.
type StageEvidenceTrigger struct {
	pool  *pgxpool.Pool
	deals *deals.Store
	own   deals.OwnDomainReader
	// read queues the MODEL's half of the ledger — the reading of what was
	// actually said, against this deal's criteria. Nil is a legal composition
	// (a deployment with no model lane) and skips silently: the deterministic
	// evidence below is what such an installation gets, and it is still true.
	read StageEvidenceReadEnqueuer
	// workspace answers the installation's own workspace, which the queued
	// reading runs in. A seam rather than a call, so a test can drive the
	// trigger without bootstrapping one.
	workspace func(context.Context) (ids.WorkspaceID, error)
	// propose asks whether the evidence just written completes a stage's
	// checklist. Nil is a legal composition and skips silently, exactly as a
	// missing model lane does: an installation composing no proposer still gets
	// a true evidence ledger, and the criteria simply go unread by the policy.
	propose stageProposer
	log     *slog.Logger
}

// stageProposer is the proposing half, as this trigger takes it. An interface
// rather than the concrete type, so the evidence lane depends on the question
// being asked and not on how a card gets staged.
type stageProposer interface {
	Propose(ctx context.Context, dealID ids.DealID) (bool, error)
}

// NewStageEvidenceTrigger builds the trigger over an explicit store and
// domain reader, which is what lets a test drive it with a fixed domain list.
func NewStageEvidenceTrigger(
	pool *pgxpool.Pool, store *deals.Store, own deals.OwnDomainReader,
	read StageEvidenceReadEnqueuer, propose stageProposer, log *slog.Logger,
) *StageEvidenceTrigger {
	return &StageEvidenceTrigger{
		pool: pool, deals: store, own: own, read: read, propose: propose,
		workspace: identity.NewService(pool).InstallationWorkspace,
		log:       log,
	}
}

// StageEvidenceDeals and StageEvidenceDomains are the two halves the worker
// hands NewStageEvidenceTrigger.
//
// Two exported helpers rather than one assembling factory, so cmd/worker names
// the constructor itself: the consumersubscribed gate traces a starter to the
// consumer it builds by the constructor's own name, and a factory wrapping it
// would leave this consumer reading as unsubscribed.
//
// The seam between the two modules lives HERE rather than in either of them —
// deals judges authorship and capture owns the domains, and a module never
// imports a sibling.
func StageEvidenceDeals(pool *pgxpool.Pool) *deals.Store {
	return deals.NewStore(InstallationDB(pool), DealsInstallation())
}

// StageEvidenceDomains answers capture's own-domain list as the seam deals
// takes, so an authorship judgement can tell a colleague from a customer.
func StageEvidenceDomains(pool *pgxpool.Pool) ownDomainReader {
	return ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))}
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil so the group keeps flowing.
func (t *StageEvidenceTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	// Older than the window is replayed history, not news. Writing evidence
	// from it would place a row against a stage the deal reached long
	// afterwards, so skipping is nil rather than an error — an error would
	// redeliver it forever.
	if time.Since(env.OccurredAt) > stageEvidenceBacklogWindow {
		return nil
	}
	// Bound HERE rather than at the write, because the READS need it too: the
	// activity lookup composes the timeline's audience clause, which asks who
	// is calling. Nobody asked for this row — the trigger acts on the system's
	// behalf throughout — and the ORIGINATING request's correlation id rides
	// along so the record that caused this and the evidence it produced are
	// recoverable as one trace. Without the id Emit refuses and the bus
	// redelivers forever.
	ctx = principal.WithCorrelationID(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   stageEvidenceActor,
	}), env.Trace.CorrelationID)
	switch env.Type {
	case eventContractStatusChanged:
		return t.onContractStatus(ctx, env)
	case eventActivityCaptured:
		return t.onActivityCaptured(ctx, env)
	case eventActivityUpdated:
		return t.onActivityUpdated(ctx, env)
	}
	return nil
}

// queueReading asks the model's half of the ledger to read this activity
// against the deal's criteria.
//
// EVERY activity that reaches a deal, not only the meetings the deterministic
// arm settles: a criterion like "the buyer stated the problem in their own
// words" is settled in prose, in an ordinary email, and the deterministic
// writers have nothing to say about it. The reading dedupes by args, so a
// redelivered event queues one reading.
//
// A failure to queue is returned rather than swallowed. The deterministic
// evidence has already been written by the time this runs, so a redelivery
// re-writes nothing — the ledger is idempotent by source — and losing the
// reading silently would leave a criterion permanently unread for a reason
// nobody could see.
func (t *StageEvidenceTrigger) queueReading(
	ctx context.Context, dealID ids.DealID, activityID ids.UUID,
) error {
	if t.read == nil {
		return nil
	}
	// RESOLVED, not read off the context. A bus envelope carries no workspace
	// and the subscriber binds none, so storekit.MustWorkspace answers the
	// zero id here — it reports no error for an unbound context — and the
	// worker would refuse every job this queued. The installation's own
	// workspace is what every other trigger path binds (InstallationDB), and
	// it is what the reading must run in.
	wsID, err := t.workspace(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, t.pool, func(tx pgx.Tx) error {
		return t.read.EnqueueTx(ctx, tx, StageEvidenceReadArgs{
			Workspace:  wsID.UUID,
			DealID:     dealID.UUID,
			ActivityID: activityID,
		}, stageEvidenceReadInsertOpts())
	})
}

// onContractStatus writes document_signed when a contract turns active.
//
// Only the transition INTO active. A contract that expires or is cancelled
// does not un-sign the document it was: the signature happened, and the
// evidence stays until a human refutes it.
func (t *StageEvidenceTrigger) onContractStatus(ctx context.Context, env events.Envelope) error {
	var payload crmcontracts.PublicEventContractStatusChanged
	if !t.readPayload(ctx, env, &payload) {
		return nil
	}
	if payload.ToStatus != "active" {
		return nil
	}
	dealID, err := t.dealOfContract(ctx, env.Entity.ID)
	if err != nil {
		// A contract naming no deal is a company-level agreement. It
		// settles no deal's criteria, which is an ordinary outcome.
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	// Both sides sign, so a signature is nobody's word — it is a recorded fact,
	// which is why document_signed is not settled by authorship. The side
	// recorded is unknown rather than a claimed buyer: the contract row says
	// the agreement is active and does not say who moved it there.
	return t.write(ctx, deals.DeterministicClaim{
		DealID:     dealID,
		Kind:       deals.CriterionDocumentSigned,
		SourceType: deals.SourceContract,
		SourceID:   env.Entity.ID,
		AuthorSide: deals.AuthorUnknown,
		ObservedAt: env.OccurredAt,
	})
}

// onActivityCaptured takes one captured activity through both halves of the
// ledger.
//
// EVERY kind reaches here, not only meetings. The deterministic half settles
// nothing on an email — judgeActivity's own kind check refuses it — but the
// model's half is asked about it, because a criterion like "the buyer stated
// the problem in their own words" is settled in ordinary prose and no record
// column says so.
func (t *StageEvidenceTrigger) onActivityCaptured(ctx context.Context, env events.Envelope) error {
	var payload crmcontracts.PublicEventActivityCaptured
	if !t.readPayload(ctx, env, &payload) {
		return nil
	}
	return t.judgeActivity(ctx, env.Entity.ID)
}

// onActivityUpdated looks again when an edit changed one of the two things
// that decide what this activity settles.
//
// A MEETING STATUS change is the first, and without it the feature is nearly
// inert. Capture never populates meeting_status — it writes the row from the
// calendar and leaves the column NULL — so a synced meeting arrives with no
// proof it took place, and the rep marking it held afterwards is the moment it
// acquires one. That moment emits activity.updated and nothing else.
//
// A RELINK is the second. An email captured before anybody attached it to a
// deal was skipped at capture — it reached no deal, so there were no criteria
// to read it against — and attaching it later is the moment it acquires some.
// Without this arm that text is never read at all: capture is over, and no
// other event will fire for it.
//
// Every other edit is ignored. Re-subject a meeting, move its time, assign it,
// correct a typo in the body — none of those changes whether it happened or
// which deal's criteria it speaks to, and re-judging on each would rewrite the
// ledger's observed_at for a fact that did not change.
func (t *StageEvidenceTrigger) onActivityUpdated(ctx context.Context, env events.Envelope) error {
	var payload crmcontracts.PublicEventActivityUpdated
	if !t.readPayload(ctx, env, &payload) {
		return nil
	}
	if payload.ChangedFields.MeetingStatus == nil && payload.ChangedFields.Relinked == nil {
		return nil
	}
	return t.judgeActivity(ctx, env.Entity.ID)
}

// judgeActivity queues the model's reading of an activity, and writes
// event_held where the record itself shows a meeting took place.
//
// What settles it is PROOF THE MEETING HAPPENED, judged by MeetingWasHeld: a
// transcript, or a held status with somebody from their side on it. Not
// authorship — capture stamps our own seat as the sender of a synced meeting,
// so an authorship rule rejected every real one.
//
// The author side recorded is the one AuthorSideOf computes. A meeting
// reaching this point has already proven itself by a rule that does not
// consult it; it rides along as the trail's account of who called the meeting.
func (t *StageEvidenceTrigger) judgeActivity(ctx context.Context, activityID ids.UUID) error {
	facts, err := t.activityFacts(ctx, activityID)
	if err != nil {
		// An activity reaching no deal settles nothing.
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	// The MODEL's half is asked about every activity that reaches a deal,
	// meeting or not: a criterion settled in prose is settled in an ordinary
	// email, and the deterministic arm below has nothing to say about one.
	if err := t.queueReading(ctx, facts.DealID, activityID); err != nil {
		return err
	}
	if facts.Kind != activityKindMeeting {
		return nil
	}
	// A meeting in the future has not been held. The capture path records
	// scheduled meetings too, and one settles event_held only once it happened.
	if facts.OccurredAt.After(time.Now()) {
		return nil
	}
	// THE MEETING'S OWN TIME, not the envelope's. An envelope's occurred_at is
	// stamped when the event is emitted, so a transcript of a six-month-old
	// meeting imported today carries a fresh envelope and would otherwise be
	// written as if the meeting had just happened.
	if time.Since(facts.OccurredAt) > stageEvidenceBacklogWindow {
		return nil
	}
	proof := deals.MeetingWasHeld(
		facts.MeetingStatus, facts.HasTranscript, facts.BuyerParticipants)
	if !proof.Held {
		return nil
	}
	claim := deals.DeterministicClaim{
		DealID:     facts.DealID,
		Kind:       deals.CriterionEventHeld,
		SourceType: deals.SourceActivity,
		SourceID:   activityID,
		AuthorSide: facts.AuthorSide,
		ObservedAt: facts.OccurredAt,
	}
	if proof.FromTranscript {
		// The whole transcript is the span this rests on. A deterministic
		// writer makes no claim about WHICH lines say the meeting happened —
		// the meeting happening is what the recording's existence shows — so
		// it cites the document rather than pretending to a passage it did not
		// read. The model's half narrows this to the lines that matter.
		claim.SourceLines = deals.WholeSpan(facts.TranscriptLines)
	}
	return t.write(ctx, claim)
}

//craft:ignore naked-any the destination is json.Unmarshal's, and each caller passes a different generated payload struct; a type parameter here would have to be instantiated per event type at a call site that already names the type
func (t *StageEvidenceTrigger) readPayload(ctx context.Context, env events.Envelope, into any) bool {
	if err := json.Unmarshal(env.Payload, into); err != nil {
		t.log.WarnContext(ctx, "stage evidence trigger: unreadable payload",
			"event", env.EventID.String(), "type", env.Type, "err", err)
		return false
	}
	return true
}

// activityFacts reads what the meeting rules need about one activity.
//
// The own-domain list is capture's and the judgement is deals', so the two
// meet HERE — compose is where an edge between two modules is allowed to
// exist. Both halves read inside one transaction, so the domains that decided
// the author side are the domains that counted the attendees.
func (t *StageEvidenceTrigger) activityFacts(
	ctx context.Context, activityID ids.UUID,
) (deals.ActivityAuthorship, error) {
	var out deals.ActivityAuthorship
	err := pgx.BeginFunc(ctx, t.pool, func(tx pgx.Tx) error {
		var err error
		out, err = deals.ReadActivityAuthorship(ctx, tx, activityID, t.own)
		return err
	})
	return out, err
}

// dealOfContract answers the deal a contract is bound to.
func (t *StageEvidenceTrigger) dealOfContract(
	ctx context.Context, contractID ids.UUID,
) (ids.DealID, error) {
	var dealID ids.DealID
	err := pgx.BeginFunc(ctx, t.pool, func(tx pgx.Tx) error {
		var err error
		dealID, err = deals.DealOfContract(ctx, tx, contractID)
		return err
	})
	return dealID, err
}

// write records one claim, on the context HandleEvent already bound to the
// system principal and the originating correlation id.
//
// captured_by therefore names the system, which is what the audit trail should
// say happened: no human asked for this row, and binding the last human to
// touch the deal would put their name on an observation they never made.
func (t *StageEvidenceTrigger) write(
	ctx context.Context, claim deals.DeterministicClaim,
) error {
	written, err := t.deals.RecordDeterministicEvidence(ctx, claim)
	if err != nil {
		return err
	}
	if written == 0 {
		// The claim was already on the ledger. Nothing changed, so the
		// checklist stands exactly where the last write left it and re-asking
		// would put the same card up twice.
		return nil
	}
	t.log.InfoContext(ctx, "stage evidence recorded",
		"deal", claim.DealID.String(), "kind", string(claim.Kind), "rows", written)
	return t.proposeStageMove(ctx, claim.DealID)
}

// proposeStageMove asks whether the evidence just written finishes a stage.
//
// A FAILURE IS RETURNED, so the bus redelivers. Nothing else would ever come
// back to it: the evidence is committed, no job is queued for the proposal, and
// the next claim on this deal may be months away or never — so swallowing the
// error loses the card for a deal whose criteria are, right now, all met.
//
// Redelivery is safe on both halves. The evidence write is ON CONFLICT DO
// NOTHING and finds its own row; the staging joins the pending card under its
// identity rather than adding a second. So a redelivered event re-proposes the
// same move onto the same card.
func (t *StageEvidenceTrigger) proposeStageMove(ctx context.Context, dealID ids.DealID) error {
	if t.propose == nil {
		return nil
	}
	staged, err := t.propose.Propose(ctx, dealID)
	if err != nil {
		return fmt.Errorf("propose the stage move this evidence completes: %w", err)
	}
	if staged {
		t.log.InfoContext(ctx, "stage move proposed", "deal", dealID.String())
	}
	return nil
}
