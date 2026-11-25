// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package assurancepair holds the pairs of open deals the input check asks
// about one buying motion, and the reader's view of such a finding. A subpackage
// of compose because the root package may not grow; it imports nothing from the
// root.
package assurancepair

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/dealseat"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// assurancePairSubjectsSQL finds the pairs. Evidence counts only when it is
// live and judged to belong to both deals.
func assurancePairSubjectsSQL(narrowed bool) string {
	only := ""
	if narrowed {
		only = " AND (a.id = ANY($1) OR b.id = ANY($1))"
	}
	return `
	SELECT a.id, b.id, a.owner_id, a.amount_minor, a.currency,
	       a.pipeline_id, b.pipeline_id, a.expected_close_date, b.expected_close_date,
	       shared.activities, stakeholders.contacts
	  FROM deal a
	  JOIN deal b ON b.company_id = a.company_id AND (b.created_at, b.id) < (a.created_at, a.id)
	 CROSS JOIN LATERAL (
	       SELECT count(DISTINCT la.activity_id) AS activities
	         FROM activity_link la
	         JOIN activity_link lb ON lb.activity_id = la.activity_id AND lb.deal_id = b.id
	         JOIN activity x ON x.id = la.activity_id
	        WHERE la.deal_id = a.id AND x.archived_at IS NULL AND x.kind IN ('email', 'meeting')
	          AND ` + relstrength.VisibleToAllSQL("x") + `
	          AND ` + relstrength.DealEvidenceSQL("x", "a.id") + `
	          AND ` + relstrength.DealEvidenceSQL("x", "b.id") + `) shared
	 CROSS JOIN LATERAL (
	       SELECT count(DISTINCT ra.contact_id) AS contacts
	         FROM relationship ra
	         JOIN relationship rb ON rb.contact_id = ra.contact_id AND rb.deal_id = b.id AND ` + dealseat.CurrentSQL("rb") + `
	        WHERE ra.deal_id = a.id AND ` + dealseat.CurrentSQL("ra") + `) stakeholders
	 WHERE a.company_id IS NOT NULL
	   AND a.archived_at IS NULL AND a.status = 'open' AND ` + deals.StandingSQL("a") + `
	   AND b.archived_at IS NULL AND b.status = 'open' AND ` + deals.StandingSQL("b") + `
	   AND shared.activities + stakeholders.contacts > 0` + only + `
	 ORDER BY a.id, b.id`
}

// Subjects reads every pair of open deals of one account that
// share a mail, a meeting or a stakeholder contact. Like AssuranceSubjects it
// runs unscoped; a reader's own scope is applied when the finding is read
// (WithholdHidden).
//
// Only mail and meetings every member may read count, as in the deal read: the
// finding reaches readers who could not open a restricted message.
func Subjects(ctx context.Context, tx pgx.Tx, _ time.Time, only []ids.UUID) ([]assurance.PairSubject, error) {
	var args []any
	if only != nil {
		args = append(args, only)
	}
	rows, err := tx.Query(ctx, assurancePairSubjectsSQL(only != nil), args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deal pairs to check: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.PairSubject, error) {
		var out assurance.PairSubject
		var later, earlier ids.UUID
		var owner *ids.UUID
		var currency *string
		var pipeline, otherPipeline ids.UUID
		err := row.Scan(&later, &earlier, &owner, &out.AmountMinor, &currency,
			&pipeline, &otherPipeline, &out.ExpectedClose, &out.OtherExpectedClose,
			&out.SharedActivities, &out.SharedStakeholders)
		out.DealID, out.OtherDealID = later.String(), earlier.String()
		out.PipelineID, out.OtherPipelineID = pipeline.String(), otherPipeline.String()
		if owner != nil {
			out.Owner = owner.String()
		}
		if currency != nil {
			out.Currency = *currency
		}
		return out, err
	})
}

// slotOtherWithheld marks a pair finding that left out the second deal because
// its reader may not see it.
const slotOtherWithheld = "other_withheld"

// WithholdHidden takes the second deal out of the pair findings whose reader
// may not see it. That is its id and the close gap, which shows its close date
// through the subject's. The finding still says a likely duplicate exists.
// The merge offer follows the id, so a reader without it is offered none.
func WithholdHidden(ctx context.Context, tx pgx.Tx, found []assurance.Exception) ([]assurance.Exception, error) {
	partners := make([]map[string]any, len(found))
	var named []ids.UUID
	for i, e := range found {
		if e.Type != assurance.TypeDuplicateMotion {
			continue
		}
		if err := json.Unmarshal(e.Observed, &partners[i]); err != nil {
			return nil, fmt.Errorf("compose: reading a pair finding: %w", err)
		}
		if id, ok := partners[i][assurance.SlotOtherDealID].(string); ok {
			if parsed, err := ids.Parse(id); err == nil {
				named = append(named, parsed)
			}
		}
	}
	if len(named) == 0 {
		return found, nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, "deal", named)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(found)
	for i, observed := range partners {
		if observed == nil {
			continue
		}
		id, err := ids.Parse(fmt.Sprint(observed[assurance.SlotOtherDealID]))
		if err == nil && visible[id] {
			continue
		}
		delete(observed, assurance.SlotOtherDealID)
		delete(observed, assurance.SlotCloseGapDays)
		observed[slotOtherWithheld] = true
		if out[i].Observed, err = json.Marshal(observed); err != nil {
			return nil, err
		}
	}
	return out, nil
}
