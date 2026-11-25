// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadMergeCarries are the statements that keep a merged-away lead in its
// initiatives, as the survivor. Where both leads hold a place in one
// initiative, the survivor's stands and the retired row is dropped. The
// tables allow one member row per initiative, and one open enrolment.
var leadMergeCarries = []struct {
	part, stmt string
	moved      bool
}{
	{"members", `DELETE FROM sales_initiative_member a
	  WHERE a.record_type = 'lead' AND a.record_id = @source AND EXISTS (
	    SELECT 1 FROM sales_initiative_member b
	     WHERE b.initiative_id = a.initiative_id AND b.record_type = 'lead' AND b.record_id = @target)
	 RETURNING initiative_id, NULL::uuid, reason, entered_at, NULL::timestamptz`, false},
	{"members", `UPDATE sales_initiative_member SET record_id = @target
	  WHERE record_type = 'lead' AND record_id = @source
	 RETURNING initiative_id, NULL::uuid, reason, entered_at, NULL::timestamptz`, true},
	{"enrolments", `DELETE FROM sales_initiative_enrolment a
	  WHERE a.record_type = 'lead' AND a.record_id = @source AND a.left_at IS NULL AND EXISTS (
	    SELECT 1 FROM sales_initiative_enrolment b
	     WHERE b.initiative_id = a.initiative_id AND b.record_type = 'lead' AND b.record_id = @target AND b.left_at IS NULL)
	 RETURNING initiative_id, id, reason, joined_at, left_at`, false},
	{"enrolments", `UPDATE sales_initiative_enrolment SET record_id = @target
	  WHERE record_type = 'lead' AND record_id = @source
	 RETURNING initiative_id, id, reason, joined_at, left_at`, true},
}

// placesByInitiative holds row images by initiative, then by member or
// enrolment.
type placesByInitiative map[ids.UUID]map[string][]map[string]any

// leadPlace is one member or enrolment row of a lead, as a merge found it.
type leadPlace struct {
	Initiative ids.UUID
	Enrolment  *ids.UUID
	Reason     string
	Since      time.Time
	Left       *time.Time
}

// CarryLeadMergeTx moves a merged-away lead's places in sales initiatives onto
// the surviving lead, in the merge's transaction. Each initiative it touches
// gets one audit row: the retired lead's rows before, and what the survivor
// holds of them after.
func CarryLeadMergeTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) error {
	if err := auth.Require(ctx, recordLead, principal.ActionUpdate); err != nil {
		return err
	}
	args := pgx.NamedArgs{"source": from, "target": to}
	before, after := placesByInitiative{}, placesByInitiative{}
	for _, carry := range leadMergeCarries {
		rows, err := tx.Query(ctx, carry.stmt, args)
		if err != nil {
			return fmt.Errorf("initiatives: carrying the merged-away lead's %s: %w", carry.part, err)
		}
		places, err := pgx.CollectRows(rows, pgx.RowToStructByPos[leadPlace])
		if err != nil {
			return fmt.Errorf("initiatives: carrying the merged-away lead's %s: %w", carry.part, err)
		}
		for _, place := range places {
			noteLeadPlace(before, place, carry.part, from)
			if carry.moved {
				noteLeadPlace(after, place, carry.part, to)
			}
		}
	}
	for _, initiative := range slices.SortedFunc(maps.Keys(before), func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }) {
		if _, err := storekit.Audit(ctx, tx, "update", object, initiative,
			map[string]any{"merged_lead_rows": before[initiative]},
			map[string]any{"merged_lead_rows": after[initiative], "merged_into": to}); err != nil {
			return fmt.Errorf("initiatives: auditing the merged-away lead's places: %w", err)
		}
	}
	return nil
}

// noteLeadPlace adds one row's image, naming the lead it belongs to, under its
// initiative.
func noteLeadPlace(into placesByInitiative, place leadPlace, part string, lead ids.UUID) {
	if into[place.Initiative] == nil {
		into[place.Initiative] = map[string][]map[string]any{}
	}
	into[place.Initiative][part] = append(into[place.Initiative][part], map[string]any{
		"lead": lead, "enrolment": place.Enrolment, "reason": place.Reason, "since": place.Since, "left": place.Left,
	})
}
