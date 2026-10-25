// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// endOfTime is a deadline no touch is later than, for reading all of a lead's
// remaining activity.
var endOfTime = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// ReDeriveLeadFromRemainingTouchesTx re-derives what a lead's activity caused
// after activity was archived out from under it: the score, the status step and
// the first-response stamp. It applies the live rules to the touches that
// remain (leadBehavioralSignals, ladderStepFor, isFirstResponseActivity).
//
// This is the one step DOWN the ladder, so it is not AdvanceLeadStatus.
// Only a step the system set is lowered, and never below the last status a
// human placed: a later system advance overwrote status_set_by, so the audit
// trail is where that human step survives. A lead a human placed keeps its status.
// A first-response stamp that matches no remaining touch (a human's disposition
// wrote it) stays.
func ReDeriveLeadFromRemainingTouchesTx(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, now time.Time) error {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, "lead", leadID.UUID); errors.Is(err, apperrors.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	lock, err := storekit.LockRow(ctx, tx, "lead", leadID.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := recomputeLeadScoreTx(ctx, tx, leadID, now); err != nil {
		return err
	}
	touches, err := leadTouchesFor(ctx, tx, leadID, endOfTime)
	if err != nil {
		return err
	}
	var status string
	var setBy *string
	var stamp *time.Time
	var created time.Time
	if err := tx.QueryRow(ctx, `SELECT status, status_set_by, first_response_at, created_at FROM lead WHERE id = $1`,
		leadID).Scan(&status, &setBy, &stamp, &created); err != nil {
		return err
	}
	floor, err := lastHumanPlacedStatus(ctx, tx, leadID)
	if err != nil {
		return err
	}
	p := storekit.NewPatch()
	if target, down := stepDownFor(LeadStatus(status), setBy, touches, floor); down {
		p.Set(leadStatusColumn, status, string(target))
	}
	if moved, to := restampFirstResponse(stamp, created, touches); moved {
		p.Set(firstResponseColumn, stamp, to)
	}
	if p.Empty() {
		return nil
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "lead", leadID.UUID, p.Before(), p.After())
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, leadID.UUID, crmcontracts.PublicEventLeadUpdated{
		ChangedFields: map[string]any{eventKeyDelta: p.Moved()},
	})
}

// lastHumanPlacedStatus answers the status of the newest audit entry that a
// human hand wrote to the lead's status, or "" when none did. status_set_by
// cannot say it: the store stamps "human" for every actor but the system, so
// the audit actor decides. A human counts, and so does an agent acting on a
// colleague's passport (on_behalf_of set). A passportless agent and a
// connector do not.
func lastHumanPlacedStatus(ctx context.Context, tx pgx.Tx, leadID ids.LeadID) (LeadStatus, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT after->>'status' FROM audit_log
 WHERE entity_type = 'lead' AND entity_id = $1
   AND after->>'status_set_by' = $2 AND after ? 'status'
   AND (actor_type = 'human' OR (actor_type = 'agent' AND on_behalf_of IS NOT NULL))
 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		leadID, string(crmcontracts.LeadStatusSetByLeadStatusSetByHuman)).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return LeadStatus(status), nil
}

// stepDownFor answers the status the remaining touches earn when it is below
// the current one and the system placed the current one. The earned status
// never falls under floor, the last status a human placed.
func stepDownFor(current LeadStatus, setBy *string, touches []leadResponseTouch, floor LeadStatus) (LeadStatus, bool) {
	if !current.Open() || setBy == nil || *setBy != string(crmcontracts.LeadStatusSetByLeadStatusSetBySystem) {
		return "", false
	}
	earned := LeadStatusNew
	if floor.Open() {
		earned = floor
	}
	for _, t := range touches {
		if step, ok := ladderStepFor(t); ok && earned.Advances(step) {
			earned = step
		}
	}
	return earned, earned.rung() < current.rung()
}

// restampFirstResponse moves a stamp that only an unaccepted remaining touch
// explains: to the earliest accepted touch, or to nothing when none is left.
func restampFirstResponse(stamp *time.Time, created time.Time, touches []leadResponseTouch) (bool, *time.Time) {
	if stamp == nil {
		return false, nil
	}
	var earliest *time.Time
	for _, t := range touches {
		at := firstResponseStamp(t.occurredAt, created)
		if at.Equal(*stamp) && isFirstResponseActivity(t) {
			return false, nil
		}
		if isFirstResponseActivity(t) && (earliest == nil || at.Before(*earliest)) {
			earliest = &at
		}
	}
	for _, t := range touches {
		if firstResponseStamp(t.occurredAt, created).Equal(*stamp) {
			return true, earliest
		}
	}
	return false, nil
}
