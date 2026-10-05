// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A commitment and the task it became settle together.
//
//   activity.updated, a task marked done     → its open claims settle as done
//   conversation_claim.changed, now done     → its task is completed
//
// THE TRIGGER IS THE EVENT, NOT THE WRITER, as stageevidencetrigger states:
// every path that ticks a task or settles a claim reaches the outbox through
// the write shape, so each lands here without knowing this consumer exists.
// The two halves live in two modules that may not import each other, which
// is why the edge sits in compose.
//
// Each reaction is a no-op when the other side is already done, so the pair
// cannot loop: completing the task emits activity.updated, which finds the
// claim already done and writes nothing. Reopening a task leaves its claim
// done, and dismissing a claim leaves its task alone: a commitment kept and
// then reopened is still a commitment that was kept, and a claim a reader
// judged was never made says nothing about whether the work happened.

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// commitmentSettleActor names this consumer on the audit rows it writes.
const commitmentSettleActor = "system:commitment-settle"

// eventClaimChanged is the event a claim's status change rides.
const eventClaimChanged = "conversation_claim.changed"

// claimStatusDone is the settled status both sides agree on.
const claimStatusDone = "done"

// CommitmentSettleTrigger keeps a commitment and its task settled together.
type CommitmentSettleTrigger struct {
	claims *contacts.Store
	tasks  *activities.Store
	log    *slog.Logger
}

// NewCommitmentSettleTrigger builds the consumer over the installation's stores.
func NewCommitmentSettleTrigger(pool *pgxpool.Pool, log *slog.Logger) *CommitmentSettleTrigger {
	db := InstallationDB(pool)
	return &CommitmentSettleTrigger{claims: contacts.NewStore(db), tasks: activities.NewStore(db), log: log}
}

// HandleEvent routes one envelope. An event this consumer does not act on
// answers nil so the group keeps flowing.
func (t *CommitmentSettleTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	// The system acts here, and the originating request's correlation id rides
	// along so the settlement and what caused it read as one trace.
	ctx = principal.WithCorrelationID(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: commitmentSettleActor,
	}), env.Trace.CorrelationID)
	switch env.Type {
	case eventActivityUpdated:
		var payload crmcontracts.PublicEventActivityUpdated
		if !t.readPayload(ctx, env, &payload) || payload.ChangedFields.IsDone == nil || !*payload.ChangedFields.IsDone {
			return nil
		}
		return t.settleClaimsOf(ctx, env.Entity.ID)
	case eventClaimChanged:
		var payload crmcontracts.PublicEventConversationClaimChanged
		if !t.readPayload(ctx, env, &payload) || payload.Status != claimStatusDone {
			return nil
		}
		return t.completeTaskOf(ctx, ids.UUID(payload.ClaimId))
	}
	return nil
}

// settleClaimsOf settles every open claim a completed task stands for.
func (t *CommitmentSettleTrigger) settleClaimsOf(ctx context.Context, taskID ids.UUID) error {
	claims, err := t.claims.OpenClaimsOnTask(ctx, taskID)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if err := t.claims.SettleConversationClaim(ctx, claim, claimStatusDone); err != nil {
			return err
		}
	}
	return nil
}

// completeTaskOf completes the task a claim settled as done became, if any.
func (t *CommitmentSettleTrigger) completeTaskOf(ctx context.Context, claimID ids.UUID) error {
	task, status, err := t.claims.ClaimTask(ctx, claimID)
	if err != nil || task == nil || status != claimStatusDone {
		return err
	}
	_, err = t.tasks.CompleteTask(ctx, ids.From[ids.ActivityKind](*task))
	return err
}

// readPayload decodes an envelope's payload. One that will not decode is
// logged and skipped: redelivering it would fail the same way forever.
func (t *CommitmentSettleTrigger) readPayload(ctx context.Context, env events.Envelope, into any) bool {
	if err := json.Unmarshal(env.Payload, into); err != nil {
		t.log.WarnContext(ctx, "commitment settle: unreadable payload",
			"event", env.EventID.String(), "type", env.Type, "err", err)
		return false
	}
	return true
}
