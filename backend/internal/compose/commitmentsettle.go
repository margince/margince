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
// WHOSE RIGHT IT IS. The write runs as the product, but only after asking
// whoever caused the event whether they could make it themselves: ticking
// a task you hold must not settle a claim on a contact you may not update, and
// settling a claim must not complete a colleague's task you may not change.
// A human answers for themselves, an agent or connector for the human it acts
// for; an agent acting for nobody settles nothing. The write is made on that
// human's behalf, so the event it emits names them again and a settlement it
// causes in turn is held to the same human's rights. A change the product made
// on nobody's behalf is followed, because the product's own passes are what
// the rules above already govern.
//
// Settling a claim also asks what settling it directly asks: that the
// conversation it was quoted from is still the human's to read.
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
	"time"

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
		if !t.readPayload(ctx, env, &payload) || payload.ChangedFields.IsDone == nil || !*payload.ChangedFields.IsDone {
			return nil
		}
		return t.settleClaimsOf(ctx, env.Actor, env.Entity.ID)
	case eventClaimChanged:
		var payload crmcontracts.PublicEventConversationClaimChanged
		if !t.readPayload(ctx, env, &payload) || payload.Status != claimStatusDone {
			return nil
		}
		return t.completeTaskOf(ctx, env.Actor, ids.UUID(payload.ClaimId), env.OccurredAt)
	}
	return nil
}

// onBehalfOfOrigin is the context the write runs under: the product, acting
// for the human behind the event when there is one, so the event the write
// emits carries them forward.
func onBehalfOfOrigin(ctx context.Context, by events.Actor) context.Context {
	seat, ok := originSeat(by)
	if !ok {
		return ctx
	}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: commitmentSettleActor, OnBehalfOf: seat,
	})
}

// settleClaimsOf settles every open claim a completed task stands for, on
// each contact whoever completed it could update.
func (t *CommitmentSettleTrigger) settleClaimsOf(ctx context.Context, by events.Actor, taskID ids.UUID) error {
	claims, err := t.claims.OpenClaimsOnTask(ctx, taskID)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		allowed, err := t.originMayWrite(ctx, by, string(recordTypeContact), claim.Contact, &claim.Source)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}
		if err := t.claims.SettleConversationClaim(onBehalfOfOrigin(ctx, by), claim.ID, claimStatusDone); err != nil {
			return err
		}
	}
	return nil
}

// completeTaskOf completes the task a claim settled as done became, if
// whoever settled the claim could complete that task themselves.
func (t *CommitmentSettleTrigger) completeTaskOf(
	ctx context.Context, by events.Actor, claimID ids.UUID, settledAt time.Time,
) error {
	task, status, err := t.claims.ClaimTask(ctx, claimID)
	if err != nil || task == nil || status != claimStatusDone {
		return err
	}
	allowed, err := t.originMayWrite(ctx, by, string(recordTypeActivity), *task, nil)
	if err != nil || !allowed {
		return err
	}
	_, err = t.tasks.CompleteTask(onBehalfOfOrigin(ctx, by), ids.From[ids.ActivityKind](*task), settledAt)
	return err
}

// originMayWrite asks whether whoever caused the event could change this row
// themselves: the object grant to update it and write authority over the row,
// asked as them. evidence, when set, is the conversation a claim was quoted
// from, which they must still be able to read.
func (t *CommitmentSettleTrigger) originMayWrite(
	ctx context.Context, by events.Actor, table string, id ids.UUID, evidence *ids.UUID,
) (bool, error) {
	seat, ok := originSeat(by)
	if !ok {
		// The product acting for nobody is followed; an agent acting for
		// nobody is not.
		return by.Type == string(principal.PrincipalSystem), nil
	}
	ws, err := t.identity.InstallationWorkspace(ctx)
	if err != nil {
		return false, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	rbac, seatType, err := t.identity.EffectiveAuthority(ctx, ws.UUID, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("commitment settle: resolving who caused the change: %w", err)
	}
	asThem := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + seat.String(), UserID: seat,
		SeatType: seatType, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions,
	})
	if err := auth.Require(asThem, table, principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	err = database.WithWorkspaceTx(asThem, t.pool, func(tx pgx.Tx) error {
		if evidence != nil {
			if err := auth.EnsureActivityContentVisibleLive(asThem, tx, *evidence); err != nil {
				return err
			}
		}
		// An activity's write rule is its own: its audience, not an owner.
		if table == string(recordTypeActivity) {
			return auth.EnsureActivityWritableIn(asThem, tx, id, true)
		}
		return auth.EnsureWritableLive(asThem, tx, table, id)
	})
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// originSeat is the human behind an event's actor: the human themselves, or
// the one an agent, a connector or this consumer acted for.
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
