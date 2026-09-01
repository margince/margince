// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The consumer that lets an introduction close itself.
//
// `replied` is the outcome the whole workflow is for, and it is the one status
// no endpoint can set. A checkbox would make the product's best number the one
// claim nobody had evidence for — every rep who ever wanted a green row could
// produce one. So it is reached here or not at all, from a message the contact
// actually sent.
//
// It lives in compose because the question crosses two modules: activities owns
// the message and who was on it, introductions owns the ask. Neither imports
// the other, so the edge is injected here.
//
// WHAT QUALIFIES, and why each clause is load-bearing:
//
//   - The activity was CAPTURED BY A CONNECTOR, not logged by a person. This is
//     the security clause, and without it the feature is a privilege
//     escalation: a member holding activity:create can log an "inbound" message
//     naming any contact they can see, at any occurred_at they choose, and the
//     writer stamps that contact as its sender (activities/participantlog.go).
//     Running as PrincipalSystem, this consumer would then close every ask
//     about that contact — including asks the member is not party to and could
//     not otherwise read or write.
//
//     The test is `captured_by`, and nothing weaker holds. `source_system` is
//     settable on the public log endpoint, so a body carrying
//     `source_system: gmail` forges that in one line. `captured_by` comes from
//     the AUTHENTICATED principal through storekit.CapturedBy — a connector's
//     is `connector:…`, a person's is `human:…` — so it is provenance a caller
//     cannot assert. capture/sinkprovenance.go states the same rule for the
//     same reason.
//   - The activity is INBOUND. An outbound mail is the rep writing to the
//     contact, which is the opposite of the contact answering.
//   - The contact is the SENDER, not merely present. Being cc'd on a colleague's
//     mail puts a person on the participant list without their having written a
//     word, and counting that would close an ask on somebody else's reply.
//   - It happened AFTER the handshake. A thread carries months of history, and
//     capture backfills it. Without this clause the first import of an existing
//     correspondence would mark every introduction ever made as answered, using
//     mail sent before anyone was introduced.
//
// WHAT THIS DELIBERATELY DOES NOT CLAIM. A connector delivers what a mail
// server handed it, and an unauthenticated From header is a header its sender
// typed. So `replied` means "a message arrived that this workspace's own
// capture attributes to the contact, after the introduction" — which is what
// the status is for, and why the evidence id travels on both the row and the
// event: the claim stays checkable against the message it rests on. Making it
// stronger is a question for capture, which owns sender authentication on
// behalf of every consumer, rather than one this consumer answers alone.
//
// Idempotency is the ask's own status: the UPDATE matches only from `introduced`
// or `name_dropped`, so a redelivered event and the second message in a thread
// both find nothing to do. events.Dedupe sits in front of that as a cache, never
// as the guarantee — it marks AFTER the effect, so a crash in that window
// replays.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The two outbox entity types this consumer reacts to: the message, and the
// person a message's sender was promoted into after the fact.
const (
	introAdvanceEntityActivity = "activity"
	introAdvancePersonEntity   = "person"
)

// systemIntroAdvanceActor names this consumer in every audit row it causes, so
// a reply the product recorded is told apart from one a person asserted.
const systemIntroAdvanceActor = "system:intro-advance"

// connectorCapturedPrefix is how a connector's provenance reads in
// activity.captured_by: capture stamps `connector:<name>[:<user>]` from the
// authenticated principal (capture/sinkprovenance.go). A person's row reads
// `human:<id>` and never matches.
//
// Built from principal.PrincipalConnector rather than typed as a literal, so
// renaming that constant breaks this expression at compile time. A hand-typed
// literal would instead go on compiling while matching nothing, and a clause
// matching nothing here is a lane that closes no introductions and reports no
// fault.
const connectorCapturedPrefix = string(principal.PrincipalConnector) + ":"

// IntroAdvance closes introductions the contact has answered.
//
// It carries no clock. The instant a reply is recorded at is the message's own
// `occurred_at`, never now(): a mail sent on Friday and captured on Monday was
// answered on Friday, and stamping the sync time would date every reply to
// whenever the mailbox happened to poll.
type IntroAdvance struct {
	pool *pgxpool.Pool
	asks *introductions.Store
	log  *slog.Logger
}

// NewIntroAdvance builds the consumer over the ask store.
func NewIntroAdvance(
	pool *pgxpool.Pool, asks *introductions.Store, log *slog.Logger,
) *IntroAdvance {
	return &IntroAdvance{pool: pool, asks: asks, log: log}
}

// HandleEvent routes one envelope. Anything else answers nil, so the consumer
// group keeps flowing rather than wedging on traffic this consumer ignores.
//
// TWO arms, because the message and the person who sent it do not arrive
// together. Capture commits the activity and its address-only participant, then
// promotes that address to a person in a SEPARATE transaction afterwards
// (capture/sinkensure.go says so: "Runs after the capture transaction
// committed"). So the activity arm can run before the sender is a person, find
// nobody, and acknowledge — losing a real reply with nothing scheduled to
// notice. The person arm is the repair, and it is the same shape
// cg:cohort-promote uses for the same race.
//
// The person arm covers a SECOND race for free, which is why it is deliberately
// keyed on the entity rather than on one event type. A message can also arrive
// while the handshake transaction is still open: the ask is not yet introduced,
// so nothing is awaiting a reply and the activity arm correctly does nothing.
// intro_request.completed rides on the PERSON entity (public-events.yaml), so
// the handshake committing is itself an event this arm consumes — it re-reads
// the contact's mail and finds the message that arrived early.
func (a *IntroAdvance) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID == ids.Nil {
		return nil
	}
	captured := env.Entity.Type == introAdvanceEntityActivity &&
		env.Type == string(crmcontracts.ActivityCaptured)
	promoted := env.Entity.Type == introAdvancePersonEntity
	if !captured && !promoted {
		return nil
	}
	ws, err := InstallationDB(a.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = a.advanceContext(ctx, env, ws.UUID)
	if promoted {
		return a.answerBacklogFor(ctx, env.Entity.ID)
	}

	// Who sent this, and when. A message with no inbound sender answers no
	// introduction, and the great majority of captured activity is exactly
	// that — so this read is the filter, before any ask is looked at.
	senders, occurredAt, err := a.inboundSenders(ctx, env.Entity.ID)
	if err != nil {
		return err
	}
	for _, personID := range senders {
		if err := a.answerAsksTo(ctx, personID, env.Entity.ID, occurredAt); err != nil {
			return err
		}
	}
	return nil
}

// answerBacklogFor re-checks the mail a contact has ALREADY sent, for the asks
// still waiting on them.
//
// This is the repair arm. A message captured before its sender was a person
// left no trace the activity arm could act on, and nothing else would ever look
// at it again — the reply would be lost permanently while the ask sat reading
// unanswered.
//
// Bounded, and cheap in the ordinary case: it runs only when an ask about this
// contact is actually awaiting a reply, which is rare, and the query then reads
// their qualifying mail since the earliest such handshake rather than their
// whole history.
func (a *IntroAdvance) answerBacklogFor(ctx context.Context, personID ids.UUID) error {
	pending, err := a.asks.AwaitingReply(ctx, personID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	earliest := pending[0].Since
	for _, ask := range pending[1:] {
		if ask.Since.Before(earliest) {
			earliest = ask.Since
		}
	}
	activityID, occurredAt, err := a.firstAnswerSince(ctx, personID, earliest)
	if err != nil || activityID == ids.Nil {
		return err
	}
	return a.answerAsksTo(ctx, personID, activityID, occurredAt)
}

// firstAnswerSince finds the EARLIEST qualifying message this contact sent after
// an instant, or the zero id when they have sent none.
//
// Earliest rather than latest: the ask was answered when they first wrote back,
// and dating it from their most recent mail would overstate how long the
// introduction took to land. It carries every clause the activity arm's read
// carries, because the repair must admit exactly what the live path admits —
// a backlog pass with a looser rule would close asks the live one refused.
func (a *IntroAdvance) firstAnswerSince(
	ctx context.Context, personID ids.UUID, since time.Time,
) (ids.UUID, time.Time, error) {
	var activityID ids.UUID
	var occurredAt time.Time
	err := a.pool.QueryRow(ctx, `
		SELECT a.id, a.occurred_at
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id
		 WHERE p.person_id = $1
		   AND a.occurred_at > $2
		   AND a.archived_at IS NULL
		   AND a.captured_by LIKE $3
		   AND a.direction = 'inbound'
		   AND p.role = 'from'
		 ORDER BY a.occurred_at
		 LIMIT 1`, personID, since, connectorCapturedPrefix+"%").Scan(&activityID, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// They have written nothing since the introduction, which is the
		// ordinary state of an ask still waiting.
		return ids.Nil, time.Time{}, nil
	}
	if err != nil {
		return ids.Nil, time.Time{}, fmt.Errorf(
			"intro-advance: reading what %s has written since %s: %w", personID, since, err)
	}
	return activityID, occurredAt, nil
}

// inboundSenders reads the contacts who SENT this activity, and when it
// happened.
//
// Role `from` and direction `inbound` are both required, and they are not the
// same test: an outbound mail also has a `from`, and an inbound one also has
// `to` rows. Only their conjunction is "this contact wrote to us".
//
// A list rather than one id because a captured message can name more than one
// person as its sender — a shared mailbox resolving to two contacts, an
// imported thread whose headers merge. Each is asked about separately, so an
// ambiguous sender advances the asks it genuinely answers rather than none.
func (a *IntroAdvance) inboundSenders(
	ctx context.Context, activityID ids.UUID,
) ([]ids.UUID, time.Time, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT p.person_id, a.occurred_at
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id
		 WHERE a.id = $1
		   AND a.archived_at IS NULL
		   AND a.captured_by LIKE $2
		   AND a.direction = 'inbound'
		   AND p.role = 'from'
		   AND p.person_id IS NOT NULL`, activityID, connectorCapturedPrefix+"%")
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("intro-advance: reading who sent %s: %w", activityID, err)
	}
	defer rows.Close()
	var senders []ids.UUID
	var occurredAt time.Time
	for rows.Next() {
		var personID ids.UUID
		if err := rows.Scan(&personID, &occurredAt); err != nil {
			return nil, time.Time{}, fmt.Errorf("intro-advance: reading a sender: %w", err)
		}
		senders = append(senders, personID)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("intro-advance: reading who sent %s: %w", activityID, err)
	}
	return senders, occurredAt, nil
}

// answerAsksTo marks every ask about this contact that this message answers.
//
// The `Since` comparison is what keeps a backfilled thread from closing asks
// wholesale: capture imports years of correspondence at once, and every one of
// those messages is inbound and from the contact. Only a message that arrived
// after the handshake can be an answer to it.
func (a *IntroAdvance) answerAsksTo(
	ctx context.Context, personID, activityID ids.UUID, occurredAt time.Time,
) error {
	pending, err := a.asks.AwaitingReply(ctx, personID)
	if err != nil {
		return err
	}
	for _, ask := range pending {
		if !occurredAt.After(ask.Since) {
			continue
		}
		replied, err := a.asks.RecordReply(ctx, ask.ID, activityID, occurredAt)
		if err != nil {
			return err
		}
		if replied {
			a.log.InfoContext(ctx, "intro-advance: a contact answered an introduction",
				"intro_request", ask.ID.String(), "activity", activityID.String())
		}
	}
	return nil
}

// advanceContext binds the workspace, the trace and the system actor this
// consumer runs as. A subscriber carries none of them: without the actor the
// first governed call fails, and without the correlation id the audit row this
// causes has no link back to the message that caused it.
func (a *IntroAdvance) advanceContext(
	ctx context.Context, env events.Envelope, ws ids.UUID,
) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   systemIntroAdvanceActor,
	})
}
