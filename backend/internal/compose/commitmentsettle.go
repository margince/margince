// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Events naming a human run with that human's current authority. Audited
// settlement times keep replay from completing a task someone reopened later.
// Dismissal retires proposals without claiming their tasks were performed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
const (
	claimStatusDone      = "done"
	claimStatusDismissed = "dismissed"
)

// CommitmentSettleTrigger retires work from settled promises.
type CommitmentSettleTrigger struct {
	db        *database.DB
	approvals *approvals.Service
	claims    *contacts.Store
	tasks     *activities.Store
	identity  *identity.Service
	log       *slog.Logger
}

// NewCommitmentSettleTrigger builds the consumer over the installation's stores.
func NewCommitmentSettleTrigger(pool *pgxpool.Pool, log *slog.Logger) *CommitmentSettleTrigger {
	db := InstallationDB(pool)
	return &CommitmentSettleTrigger{
		db: db, approvals: NewApprovalsService(db), claims: contacts.NewStore(db), tasks: activities.NewStore(db),
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
		if !readSettlePayload(ctx, t.log, env, &payload) || (payload.Status != claimStatusDone && payload.Status != claimStatusDismissed) {
			return nil
		}
		return t.applyClaimSettlement(ctx, env.Actor, ids.UUID(payload.ClaimId), env.Trace.AuditLogID)
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
	task, err := t.tasks.GetActivity(asThem, ids.From[ids.ActivityKind](taskID), storekit.IncludeArchived)
	if refused(err) {
		t.log.WarnContext(ctx, "completed commitment task no longer readable", "task", taskID, "err", err)
		return nil
	}
	if err != nil {
		return err
	}
	if task.Kind != bulkParamTask || task.IsDone == nil || !*task.IsDone {
		return nil
	}
	claims, err := t.claims.OpenClaimsOnTask(asThem, taskID)
	if refused(err) {
		t.log.WarnContext(ctx, "completed task's commitments no longer readable", "task", taskID, "err", err)
		return nil
	}
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if err := t.claims.SettleConversationClaim(asThem, claim.ID, claimStatusDone); err != nil {
			if refused(err) {
				t.log.WarnContext(ctx, "commitment claim settlement refused", "claim", claim.ID, "err", err)
				continue
			}
			return err
		}
	}
	return t.withdrawCompletedTaskOffers(asThem, task)
}

// A dismissed promise retires its proposal without claiming its task was done.
func (t *CommitmentSettleTrigger) applyClaimSettlement(ctx context.Context, by events.Actor, claimID, settlement ids.UUID) error {
	asThem, ok, err := t.actingAs(ctx, by)
	if err != nil || !ok {
		return err
	}
	settled, err := t.claims.ReadClaimSettlement(asThem, claimID, settlement)
	if refused(err) {
		t.log.WarnContext(ctx, "commitment settlement no longer readable", "claim", claimID, "err", err)
		return nil
	}
	if err != nil || settled == nil || (settled.Status != claimStatusDone && settled.Status != claimStatusDismissed) {
		return err
	}
	if settled.Status == claimStatusDone && settled.TaskID != nil {
		if _, err := t.completeSettledClaimTask(asThem, *settled); err != nil {
			if refused(err) {
				t.log.WarnContext(ctx, "commitment task completion refused", "claim", claimID, "err", err)
				return nil
			}
			return err
		}
	}
	return t.withdrawSettledOffers(asThem, settledCommitmentIdentity{sourceID: settled.SourceID, locator: settled.Locator, claimID: &claimID, status: settled.Status, settlementID: settlement})
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
