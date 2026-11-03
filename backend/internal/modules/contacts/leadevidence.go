// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadEvidenceTriggerSQL is the trigger an activity row `a` proves for the lead
// leadRef names.
func leadEvidenceTriggerSQL(leadRef string) string {
	return `(CASE WHEN a.kind = 'meeting' THEN ` + meetingProofSQL("a", leadRef) + ` ELSE '` + string(TriggerInboundReply) + `' END)`
}

// leadEvidenceOrderSQL puts the strongest engagement first: a held meeting,
// then a booked one, then a reply, each newest first.
func leadEvidenceOrderSQL(leadRef string) string {
	return ` ORDER BY CASE ` + leadEvidenceTriggerSQL(leadRef) + ` WHEN '` + string(TriggerMeetingHeld) + `' THEN 0
	                WHEN '` + string(TriggerMeetingBooked) + `' THEN 1 ELSE 2 END, a.occurred_at DESC, a.id LIMIT 1`
}

// leadEvidenceFromSQL is the engagement that can qualify the lead leadRef
// names: a live inbound reply or a meeting that proves engagement, linked to
// that lead. The lead itself must have taken part, and it must fall inside the
// waiting horizon the Worklist keeps lead engagement current by. The lead read
// offers it and the promotion requires it, so the two agree.
func leadEvidenceFromSQL(leadRef string, asOf time.Time, arg func(any) int) string {
	return `FROM activity_link l JOIN activity a ON a.id = l.activity_id
	  WHERE l.lead_id = ` + leadRef + ` AND a.restricted_at IS NULL
	    AND ` + storekit.SourceWithinHorizonSQL("a", asOf, arg) + `
	    AND (` + storekit.InboundReplySQL("a") + `
	         OR (a.kind = 'meeting' AND ` + meetingProofSQL("a", leadRef) + ` IS NOT NULL))
	    AND ` + leadTookPartSQL(leadRef)
}

// leadTookPartSQL asks that the lead is the sender of the reply `a`, or on the
// meeting `a`. Capture files a message under every lead whose address is on it,
// so a lead copied on someone else's reply is linked without having replied.
// A message with no sender address, as older captures, hand-logged mail and
// channel messages have, names its sender in counterparty_email.
func leadTookPartSQL(leadRef string) string {
	return `EXISTS (SELECT 1 FROM lead party WHERE party.id = ` + leadRef + ` AND (
	      EXISTS (SELECT 1 FROM activity_participant ap
	               WHERE ap.activity_id = a.id AND ap.address IS NOT NULL AND NOT ap.past_party_cap
	                 AND (a.kind = 'meeting' OR ap.role = 'from')
	                 AND ` + storekit.LeadEmailMatchClause("party", "ap.address") + `)
	      OR (` + storekit.LeadEmailMatchClause("party", "a.counterparty_email") + `
	          AND (a.kind = 'meeting' OR NOT EXISTS (
	                SELECT 1 FROM activity_participant sender
	                 WHERE sender.activity_id = a.id AND sender.role = 'from' AND sender.address IS NOT NULL)))))`
}

// leadQualificationEvidenceSQL is the lead read's qualification_evidence
// column, as of the read's pinned instant.
func leadQualificationEvidenceSQL(asOf time.Time, arg func(any) int) string {
	return `(SELECT jsonb_build_object('trigger', ` + leadEvidenceTriggerSQL("lead.id") + `,
	                           'activity_id', a.id, 'occurred_at', a.occurred_at, 'occurred_precision', a.occurred_precision)
	   ` + leadEvidenceFromSQL("lead.id", asOf, arg) + leadEvidenceOrderSQL("lead.id") + `)`
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
	return fmt.Sprintf("this lead has no live engagement inside the waiting horizon that shows %s; cite it, or qualify it as human_qualify", e.Trigger)
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
	if _, needsEvidence := evidenceProving[trigger]; !needsEvidence {
		if in.EvidenceActivityID != nil {
			if err := auth.EnsureActivityVisible(ctx, tx, in.EvidenceActivityID.UUID); err != nil {
				return nil, err
			}
		}
		return in.EvidenceActivityID, nil
	}
	evidence, found, err := matchingEvidence(ctx, tx, id, in)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &PromoteEvidenceError{Trigger: trigger, Cited: in.EvidenceActivityID != nil}
	}
	return &evidence, nil
}

// matchingEvidence is the lead's strongest engagement that shows the trigger,
// narrowed to the cited activity when there is one.
func matchingEvidence(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (ids.ActivityID, bool, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	lead, shows := arg(id), arg(evidenceProving[PromoteTrigger(in.Trigger)])
	leadRef := fmt.Sprintf("$%d", lead)
	from := leadEvidenceFromSQL(leadRef, slaNow(ctx), arg)
	q := fmt.Sprintf(`SELECT a.id %s AND %s = ANY($%d)`, from, leadEvidenceTriggerSQL(leadRef), shows)
	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	q += " AND " + discover
	if in.EvidenceActivityID != nil {
		q += fmt.Sprintf(` AND a.id = $%d`, arg(in.EvidenceActivityID.UUID))
	}
	var found ids.UUID
	err = tx.QueryRow(ctx, q+leadEvidenceOrderSQL(leadRef), args...).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("resolve promotion evidence: %w", err)
	}
	return ids.From[ids.ActivityKind](found), true, nil
}

// lockEvidenceActivity takes a share lock on the activity the promotion will
// cite, before the lead lock. Writers lock an activity before the records its
// links reach, so the promotion takes them in that order too. An archive or
// relink of the activity then waits for the promotion, or the promotion waits
// and reads the result.
func lockEvidenceActivity(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (ids.ActivityID, error) {
	if _, needsEvidence := evidenceProving[PromoteTrigger(in.Trigger)]; !needsEvidence {
		return ids.ActivityID{}, nil
	}
	evidence, found, err := matchingEvidence(ctx, tx, id, in)
	if err != nil || !found {
		return ids.ActivityID{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity WHERE id = $1 FOR SHARE`, evidence); err != nil {
		return ids.ActivityID{}, fmt.Errorf("lock promotion evidence: %w", err)
	}
	return evidence, nil
}

// holdEvidenceLink keeps the cited activity filed on the lead until the
// promotion commits. Evidence other than the one locked first means it moved,
// and the promotion starts again under the right lock.
func holdEvidenceLink(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput, locked ids.ActivityID, evidence *ids.ActivityID) error {
	if _, needsEvidence := evidenceProving[PromoteTrigger(in.Trigger)]; !needsEvidence || evidence == nil {
		return nil
	}
	if *evidence != locked {
		return &promoteTargetMovedError{}
	}
	rows, err := tx.Query(ctx, `SELECT activity_id FROM activity_link WHERE activity_id = $1 AND lead_id = $2 FOR SHARE`, *evidence, id)
	if err != nil {
		return fmt.Errorf("lock promotion evidence link: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("lock promotion evidence link: %w", err)
	}
	if len(held) == 0 {
		return &PromoteEvidenceError{Trigger: PromoteTrigger(in.Trigger), Cited: in.EvidenceActivityID != nil}
	}
	return nil
}
