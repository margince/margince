// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadEvidenceTriggerSQL is the trigger an activity row `a` proves.
const leadEvidenceTriggerSQL = `(CASE WHEN a.kind = 'meeting' AND a.meeting_status = 'held' THEN 'meeting_held'
	                                 WHEN a.kind = 'meeting' THEN 'meeting_booked'
	                                 ELSE 'inbound_reply' END)`

// leadEvidenceOrderSQL puts the strongest engagement first: a held meeting,
// then a booked one, then a reply, each newest first.
const leadEvidenceOrderSQL = ` ORDER BY CASE WHEN a.kind = 'meeting' AND a.meeting_status = 'held' THEN 0
	                WHEN a.kind = 'meeting' THEN 1 ELSE 2 END, a.occurred_at DESC, a.id LIMIT 1`

// leadEvidenceFromSQL is the engagement that can qualify the lead leadRef
// names: a live inbound email or a meeting not called off, linked to that lead.
// The lead read offers it and the promotion requires it, so the two agree.
func leadEvidenceFromSQL(leadRef string) string {
	return `FROM activity_link l JOIN activity a ON a.id = l.activity_id
	  WHERE l.lead_id = ` + leadRef + ` AND a.archived_at IS NULL AND a.restricted_at IS NULL
	    AND ((a.kind = 'email' AND a.direction = 'inbound')
	         OR (a.kind = 'meeting'
	             AND (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held'))))`
}

// leadQualificationEvidenceSQL is the lead read's qualification_evidence column.
func leadQualificationEvidenceSQL() string {
	return `(SELECT jsonb_build_object('trigger', ` + leadEvidenceTriggerSQL + `,
	                           'activity_id', a.id, 'occurred_at', a.occurred_at, 'occurred_precision', a.occurred_precision)
	   ` + leadEvidenceFromSQL("lead.id") + leadEvidenceOrderSQL + `)`
}

// evidenceProving maps each engagement trigger to the read's triggers that
// show it. A held meeting was booked first, so it still proves a booking.
var evidenceProving = map[PromoteTrigger][]string{
	TriggerInboundReply:  {string(TriggerInboundReply)},
	TriggerMeetingBooked: {string(TriggerMeetingBooked), string(TriggerMeetingHeld)},
	TriggerMeetingHeld:   {string(TriggerMeetingHeld)},
}

// fieldTrigger is the promotion request's trigger field.
const fieldTrigger = "trigger"

// PromoteEvidenceError maps to 422: the lead's own activities do not show the
// engagement the trigger claims.
type PromoteEvidenceError struct {
	Trigger PromoteTrigger
	// Cited is whether the caller named the activity, which decides the field.
	Cited bool
}

func (e *PromoteEvidenceError) Error() string {
	if e.Cited {
		return fmt.Sprintf("the cited activity is not live engagement on this lead that shows %s", e.Trigger)
	}
	return fmt.Sprintf("this lead has no live engagement that shows %s; cite it, or qualify it as human_qualify", e.Trigger)
}

// FieldFault names the evidence when the caller cited some, and the trigger
// when the promotion looked for it on the lead and found none.
func (e *PromoteEvidenceError) FieldFault() (field, code, message string) {
	field = fieldTrigger
	if e.Cited {
		field = "evidence.activity_id"
	}
	return field, "evidence_does_not_show_trigger", e.Error()
}

// promotionEvidence answers the activity a promotion rests on. A human's own
// judgement needs none. Every other trigger needs engagement leadEvidenceFromSQL
// admits that still shows it; with no citation, the lead's strongest is taken.
func promotionEvidence(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (*ids.ActivityID, error) {
	trigger := PromoteTrigger(in.Trigger)
	proving, needsEvidence := evidenceProving[trigger]
	if !needsEvidence {
		if in.EvidenceActivityID != nil {
			if err := auth.EnsureActivityVisible(ctx, tx, in.EvidenceActivityID.UUID); err != nil {
				return nil, err
			}
		}
		return in.EvidenceActivityID, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	lead, shows := arg(id), arg(proving)
	q := fmt.Sprintf(`SELECT a.id %s AND %s = ANY($%d)`, leadEvidenceFromSQL(fmt.Sprintf("$%d", lead)), leadEvidenceTriggerSQL, shows)
	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	q += " AND " + discover
	if in.EvidenceActivityID != nil {
		q += fmt.Sprintf(` AND a.id = $%d`, arg(in.EvidenceActivityID.UUID))
	}
	var found ids.UUID
	err = tx.QueryRow(ctx, q+leadEvidenceOrderSQL, args...).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &PromoteEvidenceError{Trigger: trigger, Cited: in.EvidenceActivityID != nil}
	}
	if err != nil {
		return nil, fmt.Errorf("resolve promotion evidence: %w", err)
	}
	evidence := ids.From[ids.ActivityKind](found)
	return &evidence, nil
}
