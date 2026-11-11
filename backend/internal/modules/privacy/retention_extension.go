// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The retention sweep over the rows of extension tables. One policy, scope
// `extension`, governs every declared table that names an age column: anonymize
// clears a row's personal columns, erase deletes the row. The table's retention
// class is its floor, and a row linked to a held record is never due. The core
// selects, acts and audits; the unit writes nothing here.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// extensionObjectType is the object type of the policy over extension rows.
// Its rows sit in many tables, so the scope has no selector of its own.
const extensionObjectType = "extension"

// extensionScopeKey is that policy's key beside the selector keys.
const extensionScopeKey = extensionObjectType + "/"

// extensionRetentionStages is the stage the extension policy adds to a pass.
const extensionRetentionStages = 1

// extensionActions are what the policy can do to a row. Archive is not one: an
// extension row has no archived state the core could set.
var extensionActions = []string{actionAnonymize, actionErase}

// evaluateExtensionPolicy runs the extension stage: one batch, shared by every
// swept table, so the stage costs what any other stage costs.
func (s *RetentionService) evaluateExtensionPolicy(ctx context.Context, pol retentionPolicy, ref time.Time) error {
	budget := retentionBatch
	for _, t := range s.eraser.extensions.SweptTables() {
		if budget == 0 {
			return nil
		}
		clears := pol.Action == actionAnonymize
		if clears && len(t.Personal) == 0 {
			continue
		}
		var due []ids.UUID
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			w, err := s.extensionWindow(ctx, tx, t, pol, ref)
			if err != nil {
				return err
			}
			due, err = s.eraser.extensions.Due(ctx, tx, t, w, clears, budget)
			return err
		})
		if err != nil {
			return fmt.Errorf("retention extension %s: select: %w", t.Name, err)
		}
		budget -= len(due)
		for _, id := range due {
			stop, err := s.postureStops(ctx, pol, extensionScopeKey)
			if err != nil || stop {
				return err
			}
			if err := s.expireExtensionRow(ctx, pol, t, id, ref); err != nil {
				return fmt.Errorf("retention extension %s on %s: %w", t.Name, id, err)
			}
		}
	}
	return nil
}

// expireExtensionRow acts on one row in one audited transaction. The window and
// the holds are asked again in the acting statement. A row held, cleared or gone
// since the selection is left alone, and nothing is audited as done to it.
func (s *RetentionService) expireExtensionRow(ctx context.Context, pol retentionPolicy, t extdecl.Table, id ids.UUID, ref time.Time) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		w, err := s.extensionWindow(ctx, tx, t, pol, ref)
		if err != nil {
			return err
		}
		act := s.eraser.extensions.ClearDue
		if pol.Action == actionErase {
			act = s.eraser.extensions.DeleteDue
		}
		acted, err := act(ctx, tx, t, w, id)
		if err != nil || !acted {
			return err
		}
		return auditRetentionAction(ctx, tx, pol, t.Name, id)
	})
}

// extensionWindow is what a sweep of one table may take at ref. A row must be
// older than the policy's window and outside the floor of the table's class.
// A carried correspondence floor keeps its rows too.
func (s *RetentionService) extensionWindow(
	ctx context.Context, tx pgx.Tx, t extdecl.Table, pol retentionPolicy, ref time.Time,
) (extprivacy.Window, error) {
	policy, err := resolveFloorPolicyAt(ctx, tx, s.eraser.country, ref)
	if err != nil {
		return extprivacy.Window{}, err
	}
	w := extprivacy.Window{OlderThan: ref.AddDate(0, 0, -pol.RetainDays)}
	floor := strictestClass(t.Retention, ref)
	if pack, ok := jurisdiction.For(policy.Selected); ok {
		floor = classOf(pack, t.Retention)
	}
	if t.Retention == jurisdiction.CommercialCorrespondence {
		floor = policy.Current
		if carried := policy.Carried; carried != nil {
			before, since := carried.Before, carried.Class.ProtectedSince(ref)
			w.CarriedBefore, w.CarriedSince = &before, &since
		}
	}
	if protected := floor.ProtectedSince(ref); protected.Before(w.OlderThan) {
		w.OlderThan = protected
	}
	return w, nil
}

// supportsExtensionAction reports whether the extension policy can run action.
func supportsExtensionAction(action string) bool { return slices.Contains(extensionActions, action) }
