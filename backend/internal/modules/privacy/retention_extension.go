// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The retention sweep over the rows of an extension's tables. A policy names
// one declared table (`extension/<table>`) and one action, `expire`. The core
// selects, acts and audits; the unit writes nothing here.
//
// A row is due when it is older than the policy's window and outside its
// class floor. A held row is never due. Hold and floor are asked again inside
// the acting transaction, because either may change after the selection.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// extensionObjectType is the object type of a retention policy over the rows of
// an extension's table. The category names the table.
const extensionObjectType = "extension"

// sweptExtensionTable is the table a policy governs, when a unit declared it and
// gave it an age column to count from.
func (s *RetentionService) sweptExtensionTable(pol retentionPolicy) (extdecl.Table, bool) {
	if pol.ObjectType != extensionObjectType || pol.Category == nil {
		return extdecl.Table{}, false
	}
	t, declared := s.eraser.extensions.Table(*pol.Category)
	return t, declared && t.AgeColumn != ""
}

// extensionFloor is the floor a table's retention class puts in force at ref.
// Only the commercial correspondence floor can be carried over a change to a
// weaker jurisdiction, so only that class has a carried part.
func (s *RetentionService) extensionFloor(
	ctx context.Context, tx pgx.Tx, t extdecl.Table, ref time.Time,
) (jurisdiction.RetentionClass, *CarriedFloor, error) {
	policy, err := resolveFloorPolicyAt(ctx, tx, s.eraser.country, ref)
	if err != nil {
		return jurisdiction.RetentionClass{}, nil, err
	}
	if t.Retention == jurisdiction.CommercialCorrespondence {
		return policy.Current, policy.Carried, nil
	}
	if pack, ok := jurisdiction.For(policy.Selected); ok {
		return classOf(pack, t.Retention), nil, nil
	}
	return strictestClass(t.Retention, ref), nil, nil
}

// extensionWindow is what a sweep of one table may take at ref. Rows must be
// older than the policy's days and the floor, and outside a carried floor.
func (s *RetentionService) extensionWindow(
	ctx context.Context, tx pgx.Tx, t extdecl.Table, pol retentionPolicy, ref time.Time,
) (extprivacy.Window, error) {
	current, carried, err := s.extensionFloor(ctx, tx, t, ref)
	if err != nil {
		return extprivacy.Window{}, err
	}
	w := extprivacy.Window{OlderThan: ref.AddDate(0, 0, -pol.RetainDays)}
	if floor := current.ProtectedSince(ref); floor.Before(w.OlderThan) {
		w.OlderThan = floor
	}
	if carried != nil {
		before, since := carried.Before, carried.Class.ProtectedSince(ref)
		w.CarriedBefore, w.CarriedSince = &before, &since
	}
	return w, nil
}

// dueExtensionRows reads one batch of rows a policy may expire.
func (s *RetentionService) dueExtensionRows(
	ctx context.Context, t extdecl.Table, pol retentionPolicy, ref time.Time,
) ([]ids.UUID, error) {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		w, err := s.extensionWindow(ctx, tx, t, pol, ref)
		if err != nil {
			return err
		}
		due, err = s.eraser.extensions.Due(ctx, tx, t, w, retentionBatch)
		return err
	})
	return due, err
}

// expireExtensionRow ends the retention of one row in one audited transaction.
// A row held, cleared, deleted or put back under the floor since the selection
// is left alone, and nothing is audited as done to it.
func (s *RetentionService) expireExtensionRow(ctx context.Context, pol retentionPolicy, id ids.UUID) error {
	t, ok := s.sweptExtensionTable(pol)
	if !ok {
		return fmt.Errorf("retention: policy %s names no extension table that can be swept", pol.ID)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		state, err := s.eraser.extensions.State(ctx, tx, t, id)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil || state.Held || state.Expired {
			return err
		}
		w, err := s.extensionWindow(ctx, tx, t, pol, time.Now())
		if err != nil {
			return err
		}
		if due, err := s.eraser.extensions.StillDue(ctx, tx, t, w, id); err != nil || !due {
			return err
		}
		if err := s.eraser.extensions.Expire(ctx, tx, t, id); err != nil {
			return err
		}
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionExpire, t.Name, id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: actionExpire, "policy": pol.ID, fieldRetainDays: pol.RetainDays,
		})
		if err != nil {
			return err
		}
		policyID := pol.ID
		return storekit.EmitEventForEntity(ctx, tx, auditID, t.Name, id,
			retentionAppliedPayload(crmcontracts.RetentionAppliedExpire, &policyID, nil))
	})
}
