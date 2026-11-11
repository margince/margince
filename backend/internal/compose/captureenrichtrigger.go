// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Arrival events mark contacts durably before queueing the shared signature reader.
// The nightly pass remains the legacy reconciliation path.

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Untagged replay stays bounded; attributed history requests survive delayed delivery.
const captureEnrichFreshWindow = time.Hour

// CaptureEnrichTrigger records durable work for newly readable mail.
type CaptureEnrichTrigger struct {
	pool    *pgxpool.Pool
	enqueue *jobs.Runner
	log     *slog.Logger
	now     func() time.Time
}

// NewCaptureEnrichTrigger builds the trigger over an insert-only jobs runner.
func NewCaptureEnrichTrigger(pool *pgxpool.Pool, enqueue *jobs.Runner, log *slog.Logger) *CaptureEnrichTrigger {
	return &CaptureEnrichTrigger{pool: pool, enqueue: enqueue, log: log, now: time.Now}
}

// queues answers whether this envelope can have made somebody newly readable.
//
// An unreadable payload answers no rather than guessing: the reconciler covers
// whatever the event announced, and a consumer that wedged its group on one
// malformed body would stop reading every later one.
func (g *CaptureEnrichTrigger) queues(ctx context.Context, env events.Envelope) bool {
	switch env.Type {
	case eventActivityCaptured:
		// EMAIL only, decided before anything is queued. Only a mail carries a
		// signature block. A meeting or a call would queue a model-backed pass
		// with no work to do.
		var payload crmcontracts.PublicEventActivityCaptured
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			g.log.WarnContext(ctx, "capture enrich trigger: unreadable capture payload",
				"event", env.EventID.String(), "err", err)
			return false
		}
		// The generated enum, not a literal: crm.yaml owns this vocabulary, and
		// a word hand-typed here would not move when the contract does. The
		// same comparison the vCard trigger makes on the same field.
		return payload.Kind == string(crmcontracts.ActivityKindEmail)
	case contactCreatedEvent:
		// Every new contact, not only the ones a verdict minted. The event does
		// not say who created the contact, and asking would be this consumer
		// guessing at the pass's own selection: a hand-typed contact with no
		// mail simply is not a candidate, which costs one query that returns
		// nobody. Narrowing here to the capture-created case would instead mean
		// two spellings of who is due, and the quieter one wins arguments.
		return true
	case eventActivityUpdated:
		var payload crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			g.log.WarnContext(ctx, "capture enrich trigger: unreadable update payload",
				"event", env.EventID.String(), "err", err)
			return false
		}
		audience := payload.ChangedFields.Audience
		return signatureRelinkedContact(payload.ChangedFields) != nil || (audience != nil && *audience == crmcontracts.Workspace)
	}
	return false
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic. An enqueue failure comes back as an error and the bus
// redelivers — safe, because a redelivered event dedupes onto the pass the
// first delivery queued, and the nightly pass still reconciles what slips
// through.
func (g *CaptureEnrichTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if !g.queues(ctx, env) {
		return nil
	}
	if env.Trace.AuditLogID == ids.Nil && g.now().Sub(env.OccurredAt) > captureEnrichFreshWindow {
		return nil
	}
	requested, err := g.requestContacts(ctx, env)
	if err != nil || !requested {
		return err
	}
	return g.enqueue.Enqueue(ctx, CaptureEnrichArgs{Pending: true}, pendingWorkOpts(CaptureEnrichArgs{}.Kind()))
}

func signatureRelinkedContact(fields crmcontracts.PublicEventActivityChangedFields) *ids.UUID {
	if fields.Relinked == nil || fields.Relinked.EntityType != string(recordTypeContact) || ids.UUID(fields.Relinked.EntityId) == ids.Nil {
		return nil
	}
	id := ids.UUID(fields.Relinked.EntityId)
	return &id
}
