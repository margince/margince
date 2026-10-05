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
// WHOSE RIGHT IT IS. The write runs as whoever caused the event: a human as
// themselves, an agent or connector as the human it acts for. So the stores'
// own gates ask about that human inside the write's transaction — ticking a
// task you hold must not settle a claim on a contact you may not update, nor
// one quoted from a conversation you may no longer read, and settling a claim
// must not complete a task you may not change. The event the write emits names
// the same human, so a settlement it causes in turn is held to their rights
// too. An agent acting for nobody settles nothing; the product acting for
// nobody is followed, because its own passes are what the rules above govern.
//
// A HISTORY REPLAYED IS NOT RE-ENACTED. A new group starts at the stream's
// beginning, so a claim settled long ago arrives as news. A task changed after
// that settlement — reopened, most likely — is not completed again.
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
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	pool     *pgxpool.Pool
	claims   *contacts.Store
	tasks    *activities.Store
	identity *identity.Service
	log      *slog.Logger
}

// NewCommitmentSettleTrigger builds the consumer over the installation's stores.
func NewCommitmentSettleTrigger(pool *pgxpool.Pool, log *slog.Logger) *CommitmentSettleTrigger {
	db := InstallationDB(pool)
	return &CommitmentSettleTrigger{
		pool: pool, claims: contacts.NewStore(db), tasks: activities.NewStore(db),
		identity: identity.NewService(pool), log: log,
	}
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
		if !readSettlePayload(ctx, t.log, env, &payload) || payload.ChangedFields.IsDone == nil || !*payload.ChangedFields.IsDone {
			return nil
		}
		return t.settleClaimsOf(ctx, env.Actor, env.Entity.ID)
	case eventClaimChanged:
		var payload crmcontracts.PublicEventConversationClaimChanged
		if !readSettlePayload(ctx, t.log, env, &payload) || payload.Status != claimStatusDone {
			return nil
		}
		return t.completeTaskOf(ctx, env.Actor, ids.UUID(payload.ClaimId))
	}
	return nil
}

// settleClaimsOf settles every open claim a completed task stands for, on
// each contact whoever completed it could update.
func (t *CommitmentSettleTrigger) settleClaimsOf(ctx context.Context, by events.Actor, taskID ids.UUID) error {
	asThem, ok, err := t.actingAs(ctx, by)
	if err != nil || !ok {
		return err
	}
	claims, err := t.claims.OpenClaimsOnTask(ctx, taskID)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		// Settling checks the claim's own conversation as them; the contact's
		// write authority is asked here, because settling does not.
		writable, err := t.mayWriteContact(asThem, claim.Contact)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if err := t.claims.SettleConversationClaim(asThem, claim.ID, claimStatusDone); err != nil {
			if refused(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// completeTaskOf completes the task a claim settled as done became, as
// whoever settled the claim; a task they may not change stays as it is.
func (t *CommitmentSettleTrigger) completeTaskOf(ctx context.Context, by events.Actor, claimID ids.UUID) error {
	task, status, settledAt, err := t.claims.ClaimTask(ctx, claimID)
	if err != nil || task == nil || status != claimStatusDone {
		return err
	}
	asThem, ok, err := t.actingAs(ctx, by)
	if err != nil || !ok {
		return err
	}
	_, err = t.tasks.CompleteTask(asThem, ids.From[ids.ActivityKind](*task), settledAt)
	if refused(err) {
		return nil
	}
	return err
}

// actingAs is the context the write runs under: the human behind the event,
// with the authority they hold now, or the product when nobody is behind it.
// It answers false for an agent or connector acting for nobody.
func (t *CommitmentSettleTrigger) actingAs(ctx context.Context, by events.Actor) (context.Context, bool, error) {
	ws, err := t.identity.InstallationWorkspace(ctx)
	if err != nil {
		return nil, false, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	seat, ok := originSeat(by)
	if !ok {
		return ctx, by.Type == string(principal.PrincipalSystem), nil
	}
	rbac, seatType, err := t.identity.EffectiveAuthority(ctx, ws.UUID, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("commitment settle: resolving who caused the change: %w", err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + seat.String(), UserID: seat,
		SeatType: seatType, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions,
	}), true, nil
}

// mayWriteContact asks whether the acting human could update this contact.
func (t *CommitmentSettleTrigger) mayWriteContact(asThem context.Context, contact ids.UUID) (bool, error) {
	if err := auth.Require(asThem, string(recordTypeContact), principal.ActionUpdate); err != nil {
		if refused(err) {
			return false, nil
		}
		return false, err
	}
	err := database.WithWorkspaceTx(asThem, t.pool, func(tx pgx.Tx) error {
		return auth.EnsureWritableLive(asThem, tx, string(recordTypeContact), contact)
	})
	if refused(err) {
		return false, nil
	}
	return err == nil, err
}

// refused reports an answer that means "not theirs to do": the write is
// skipped, not retried, because asking again gets the same answer.
func refused(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound)
}

// originSeat is the human behind an event's actor: the human themselves, or
// the one an agent or connector acted for.
func originSeat(by events.Actor) (ids.UUID, bool) {
	if by.Type == string(principal.PrincipalHuman) {
		seat, err := ids.Parse(strings.TrimPrefix(by.ID, principal.HumanIDPrefix))
		return seat, err == nil
	}
	if by.OnBehalfOf != nil {
		return *by.OnBehalfOf, true
	}
	return ids.UUID{}, false
}

// readSettlePayload decodes an envelope's payload. One that will not decode is
// logged and skipped: redelivering it would fail the same way forever.
func readSettlePayload[P crmcontracts.PublicEventActivityUpdated | crmcontracts.PublicEventConversationClaimChanged](
	ctx context.Context, log *slog.Logger, env events.Envelope, into *P,
) bool {
	if err := json.Unmarshal(env.Payload, into); err != nil {
		log.WarnContext(ctx, "commitment settle: unreadable payload",
			"event", env.EventID.String(), "type", env.Type, "err", err)
		return false
	}
	return true
}
