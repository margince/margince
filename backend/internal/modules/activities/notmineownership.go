// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// "Not mine" is a judgement about who the work belongs to, so it ends when the
// work changes hands.
//
// A rep takes an unanswered message off their Worklist with `not_mine`. The
// judgement carries no moment — it does not become false on a Thursday — and
// until now nothing ended it either: a rep who handed a deal on and later
// inherited it back still had the thread hidden, and had to remember they once
// dismissed it.
//
// Registered as SYSTEM workflows on the ownership events of the four records a
// message can be filed under. It rides the automation engine rather than a
// consumer group of its own for the reason the engine exists: it already
// subscribes to every stream, carries the run claim that makes redelivery safe,
// and gives the reconcile a home beside the sibling that keeps a system task
// pointed at its lead's owner.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// NotMineRearmWorkflows returns the handlers that re-arm a set-aside message
// when the record it is filed under changes hands. compose registers them
// beside OwnershipWorkflows.
func NotMineRearmWorkflows(store *Store) []workflow.Handler {
	out := make([]workflow.Handler, 0, len(handOffTriggers))
	for _, trigger := range handOffTriggers {
		out = append(out, notMineRearm{store: store, trigger: trigger})
	}
	return out
}

// handOffTriggers is one trigger per record a message can be filed under.
//
// Only the DEAL has an owner event of its own; the other three announce a
// hand-off inside their open `*.updated` envelope, which is why Match below
// reads changed_fields rather than trusting the type alone. A project carries
// no owner, so no message is filed under one changing hands.
var handOffTriggers = []string{
	"deal.owner_changed",
	"person.updated",
	"organization.updated",
	"lead.updated",
}

// ownerBearing are the records that carry an owner at all — a project does not,
// and no message is filed under one changing hands.
//
// A SET rather than a map to table names: the entity type arrives on an event
// envelope, so the table it resolves to must come from a closed vocabulary
// before any statement names it. Each record's own table is its entity type,
// and the name reaches SQL through pgx.Identifier.Sanitize below.
var ownerBearing = map[datasource.EntityType]bool{
	datasource.EntityPerson:       true,
	datasource.EntityOrganization: true,
	datasource.EntityDeal:         true,
	datasource.EntityLead:         true,
}

// notMineRearm is one trigger's arm of the re-arm.
type notMineRearm struct {
	store   *Store
	trigger string
}

func (w notMineRearm) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "not_mine_rearm_on_" + w.trigger,
		Trigger: workflow.Trigger{EventType: w.trigger},
		Tier:    mcp.TierAutoExecute,
	}
}

// Match declines an update that changed no owner.
//
// `deal.owner_changed` is the hand-off itself and always matches. The other
// three ride an envelope that fires on every column, and a person's row is
// written by every enrichment pass — so running the reconcile on all of them
// would put a delete over one contact's whole timeline behind every field the
// provider fills in.
//
// It fails toward DOING the work: a payload this build cannot read as a set of
// changed fields matches, because the cost of running the reconcile for nothing
// is one statement that deletes no rows, and the cost of skipping it is a
// message that stays hidden from the person who now owns it with nothing to say
// why.
func (w notMineRearm) Match(_ context.Context, ev workflow.Event) (bool, error) {
	if w.trigger == handOffTriggers[0] {
		return true, nil
	}
	return namesTheOwner(ev.Payload), nil
}

// namesTheOwner reads an update envelope for a changed owner, in BOTH shapes the
// contract documents: the flat map a patch produces and the `{delta: {…}}` one
// the routing writes. An envelope in neither shape answers true — see Match.
func namesTheOwner(payload json.RawMessage) bool {
	if len(payload) == 0 {
		return true
	}
	var envelope struct {
		ChangedFields map[string]json.RawMessage `json:"changed_fields"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.ChangedFields == nil {
		return true
	}
	if _, named := envelope.ChangedFields["owner_id"]; named {
		return true
	}
	delta, nested := envelope.ChangedFields["delta"]
	if !nested {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(delta, &fields); err != nil {
		return true
	}
	_, named := fields["owner_id"]
	return named
}

func (notMineRearm) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	// WHICH messages is Apply's query — the envelope carries no links — so Plan
	// states the invariant, the way its ownership sibling does.
	args, err := json.Marshal(map[string]any{dispositionField: statePickedUp})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("activities: encoding the re-arm effect: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionUpdateRecord, Target: ev.Entity, Args: args,
	}}}, nil
}

func (w notMineRearm) Apply(
	ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken,
) (workflow.RunResult, error) {
	cleared, err := w.store.ClearNotMineOnHandOff(ctx, ev.Entity.Type, ev.Entity.ID)
	if err != nil {
		return workflow.RunResult{}, fmt.Errorf("re-arming set-aside messages on %s %s: %w",
			ev.Entity.Type, ev.Entity.ID, err)
	}
	if cleared == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

func (w notMineRearm) IdempotencyKey(ev workflow.Event) string {
	// The event id, not the record: a second hand-off is a second firing that
	// must run again, and keying on the record would swallow it.
	return w.Spec().Name + ":" + ev.ID.String()
}

// ClearNotMineOnHandOff re-arms every set-aside this hand-off invalidated, and
// answers how many it cleared.
//
// Reconciliation, not a delta: it reads who owns the record NOW. Only
// `deal.owner_changed` carries the previous owner at all — a claim and the three
// `*.updated` envelopes name the incoming owner and nothing else — so a handler
// built on the difference would work for one record type and quietly do nothing
// for the other three.
//
// It clears the readers who are NOT the incoming owner. Their judgement was
// about the arrangement that just ended: the rep who dismissed a thread because
// somebody else owned the deal is exactly who should see it again when it moves.
// The incoming owner's own row is left alone, because being handed a record does
// not withdraw a statement they made about it themselves — theirs is theirs to
// pick back up.
//
// A record with no owner clears nothing. An unowned record is ordinarily one
// whose routing has not run yet, and re-arming everybody's Worklist on the way
// past would be work nobody asked for.
//
// Each clearing goes through recordDisposition — the module's own write shape —
// so it carries the audit row and the `activity.disposition_recorded` event a
// reader's own pick-up carries. A bulk DELETE would move the messages and leave
// no history of what put them back.
func (s *Store) ClearNotMineOnHandOff(
	ctx context.Context, entityType datasource.EntityType, entityID ids.UUID,
) (int, error) {
	if err := onlyTheOwnershipReconcile(ctx); err != nil {
		return 0, err
	}
	if !ownerBearing[entityType] {
		// Not an error: the engine fires this on every record type it routes,
		// and one that carries no owner simply has no hand-off to answer.
		return 0, nil
	}
	table := pgx.Identifier{string(entityType)}.Sanitize()
	column := linkColumn(string(entityType))
	if column == "" {
		return 0, nil
	}
	var cleared int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var owner *ids.UUID
		if err := tx.QueryRow(ctx,
			storekit.SQLf(`SELECT owner_id FROM %s WHERE id = $1 AND archived_at IS NULL`, table),
			entityID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("activities: reading who owns the handed-off record: %w", err)
		}
		if owner == nil {
			return nil
		}
		rows, err := tx.Query(ctx, storekit.SQLf(`
			DELETE FROM activity_reader_state s
			 USING activity_link l
			 WHERE l.activity_id = s.activity_id
			   AND l.%s = $1
			   AND s.state = $2
			   AND s.reader_id <> $3
			RETURNING s.activity_id`, column),
			entityID, stateNotMine, *owner)
		if err != nil {
			return fmt.Errorf("activities: re-arming set-aside messages: %w", err)
		}
		freed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("activities: re-arming set-aside messages: %w", err)
		}
		for _, id := range freed {
			if err := s.recordDisposition(ctx, tx, ids.From[ids.ActivityKind](id), statePickedUp, nil, ""); err != nil {
				return err
			}
		}
		cleared = len(freed)
		return nil
	})
	return cleared, err
}

// onlyTheOwnershipReconcile admits the system and refuses everybody else.
//
// This is an open bulk write over rows that belong to OTHER READERS: it clears
// judgements colleagues made, on a record the caller merely names. Left open,
// any authenticated user could put a record's whole thread back on every
// colleague's Worklist at once, and each row would look exactly like one a
// hand-off produced — the same shape the approvals sweep refuses next door, and
// for the same reason.
//
// Any system principal, not one named actor. The record id here is not
// caller-chosen in the sense that matters: it comes off an event envelope the
// bus delivered, and every system principal in this tree is a job or the engine
// running one. Naming a single actor would refuse the engine the day its
// binding is renamed, which is a silent re-arm that stops happening.
func onlyTheOwnershipReconcile(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("activities: the ownership reconcile ran with no bound actor: %w",
			apperrors.ErrPermissionDenied)
	}
	if p.Type != principal.PrincipalSystem {
		return fmt.Errorf("activities: %s may not re-arm a colleague's set-aside messages — "+
			"the hand-off reconcile runs as the system: %w", p.ID, apperrors.ErrPermissionDenied)
	}
	return nil
}
