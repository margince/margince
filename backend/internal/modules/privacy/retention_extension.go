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

// evaluateExtensionPolicy runs the extension stage: one batch, so the stage
// costs what any other stage costs. Each table first gets an equal share of it.
// What a table leaves unused goes to the others in a second round, which skips a
// table that ran dry. So a table with a long backlog never keeps the next one
// from its turn, and the tables take turns at going first.
func (s *RetentionService) evaluateExtensionPolicy(ctx context.Context, pol retentionPolicy, ref time.Time) error {
	var tables []extdecl.Table
	for _, t := range s.eraser.extensions.SweptTables() {
		if pol.Action != actionAnonymize || len(t.Personal) > 0 {
			tables = append(tables, t)
		}
	}
	if len(tables) == 0 {
		return nil
	}
	tables = inTurn(tables, ref)
	budget, drained := retentionBatch, make([]bool, len(tables))
	for _, share := range []int{max(1, retentionBatch/len(tables)), retentionBatch} {
		for i, t := range tables {
			if drained[i] {
				continue
			}
			limit := min(share, budget)
			taken, stop, err := s.sweepExtensionTable(ctx, pol, t, ref, limit)
			if err != nil || stop {
				return err
			}
			drained[i] = taken < limit
			if budget -= taken; budget == 0 {
				return nil
			}
		}
	}
	return nil
}

// inTurn starts the tables at the one whose turn the pass's day is. With more
// tables than the batch has rows, a fixed order would never reach the last ones.
func inTurn(tables []extdecl.Table, ref time.Time) []extdecl.Table {
	first := int(ref.Unix()/int64(24*time.Hour/time.Second)) % len(tables)
	return append(slices.Clone(tables[first:]), tables[:first]...)
}

// sweepExtensionTable expires up to limit due rows of one table. It reports how
// many it selected, and whether the retain-only posture stopped the pass.
func (s *RetentionService) sweepExtensionTable(
	ctx context.Context, pol retentionPolicy, t extdecl.Table, ref time.Time, limit int,
) (int, bool, error) {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		w, err := s.extensionWindow(ctx, tx, t, pol, ref)
		if err != nil {
			return err
		}
		due, err = s.eraser.extensions.Due(ctx, tx, t, w, pol.Action == actionAnonymize, limit)
		return err
	})
	if err != nil {
		return 0, false, fmt.Errorf("retention extension %s: select: %w", t.Name, err)
	}
	for _, id := range due {
		stop, err := s.postureStops(ctx, pol, extensionScopeKey)
		if err != nil || stop {
			return len(due), stop, err
		}
		if err := s.expireExtensionRow(ctx, pol, t, id, ref); err != nil {
			return len(due), false, fmt.Errorf("retention extension %s on %s: %w", t.Name, id, err)
		}
	}
	return len(due), false, nil
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
func (s *RetentionService) extensionWindow(
	ctx context.Context, tx pgx.Tx, t extdecl.Table, pol retentionPolicy, ref time.Time,
) (extprivacy.Window, error) {
	policy, err := resolveFloorPolicyAt(ctx, tx, s.eraser.country, ref)
	if err != nil {
		return extprivacy.Window{}, err
	}
	return extensionWindowUnder(t, pol, policy, ref), nil
}

// extensionWindowUnder reads the floor as the core reads it for mail. The floor
// in force binds every row. A carried floor binds only rows whose age column is
// before the change that weakened it.
func extensionWindowUnder(t extdecl.Table, pol retentionPolicy, policy FloorPolicy, ref time.Time) extprivacy.Window {
	w := extprivacy.Window{OlderThan: ref.AddDate(0, 0, -pol.RetainDays)}
	if protected := extensionFloor(t, policy, ref).ProtectedSince(ref); protected.Before(w.OlderThan) {
		w.OlderThan = protected
	}
	if carried := policy.Carried; carried != nil && t.Retention == jurisdiction.CommercialCorrespondence {
		before, since := carried.Before, carried.Class.ProtectedSince(ref)
		w.CarriedBefore, w.CarriedSince = &before, &since
	}
	return w
}

// extensionFloor is the class that keeps every row of a table at ref. For
// correspondence it is the strictest floor in force, current or any entity's: a
// unit row is not tied to one entity's mail.
func extensionFloor(t extdecl.Table, policy FloorPolicy, ref time.Time) jurisdiction.RetentionClass {
	if t.Retention == jurisdiction.CommercialCorrespondence {
		return policy.strictestInForce(ref)
	}
	if pack, ok := jurisdiction.For(policy.Selected); ok {
		return classOf(pack, t.Retention)
	}
	return strictestClass(t.Retention, ref)
}

// supportsExtensionAction reports whether the extension policy can run action.
func supportsExtensionAction(action string) bool { return slices.Contains(extensionActions, action) }
