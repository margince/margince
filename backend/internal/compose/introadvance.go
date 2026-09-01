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
// Idempotency is the ask's own status: the UPDATE matches only from `introduced`
// or `name_dropped`, so a redelivered event and the second message in a thread
// both find nothing to do. events.Dedupe sits in front of that as a cache, never
// as the guarantee — it marks AFTER the effect, so a crash in that window
// replays.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// introAdvanceEntityActivity is the outbox entity type this consumer reacts to.
const introAdvanceEntityActivity = "activity"

// systemIntroAdvanceActor names this consumer in every audit row it causes, so
// a reply the product recorded is told apart from one a person asserted.
const systemIntroAdvanceActor = "system:intro-advance"

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
func (a *IntroAdvance) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID == ids.Nil || env.Entity.Type != introAdvanceEntityActivity ||
		env.Type != string(crmcontracts.ActivityCaptured) {
		return nil
	}
	ws, err := InstallationDB(a.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = a.advanceContext(ctx, env, ws.UUID)

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
		   AND a.direction = 'inbound'
		   AND p.role = 'from'
		   AND p.person_id IS NOT NULL`, activityID)
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
