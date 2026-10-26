// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingProofPage selects live machine claims that a meeting was held. A
// model's reading of a meeting rests on its text, so it is not judged here.
const meetingProofPage = `
	SELECT ev.id, ev.deal_id, ev.source_id
	  FROM deal_stage_evidence ev
	  JOIN stage_exit_criterion c ON c.id = ev.criterion_id
	 WHERE ev.source_type = $1 AND ev.extracted_by = $2 AND c.kind = $3
	   AND ev.met AND ev.refuted_at IS NULL AND ev.id > $4
	 ORDER BY ev.id LIMIT $5`

type meetingClaim struct {
	ID       ids.UUID
	DealID   ids.UUID
	SourceID ids.UUID
}

// reconcileMeetingProof judges again whether each meeting behind a live
// machine event_held claim still proves it was held, by MeetingWasHeld. A
// claim the meeting no longer makes is marked not met. It answers how many.
func (s *Store) reconcileMeetingProof(ctx context.Context, own OwnDomainReader) (int, error) {
	withdrawn := 0
	var after ids.UUID
	for {
		var page []meetingClaim
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, meetingProofPage, SourceActivity, ExtractedByDeterministic, string(CriterionEventHeld), after, evidenceAuthorshipBatch)
			if err != nil {
				return fmt.Errorf("deals: reading meeting claims to judge again: %w", err)
			}
			if page, err = pgx.CollectRows(rows, pgx.RowToStructByPos[meetingClaim]); err != nil {
				return fmt.Errorf("deals: reading meeting claims to judge again: %w", err)
			}
			for _, claim := range page {
				after = claim.ID
				held, found, err := meetingStillProvesHeld(ctx, tx, claim.SourceID, own)
				if err != nil {
					return err
				}
				if !found || held {
					continue
				}
				done, err := withdrawMeetingClaim(ctx, tx, claim)
				if err != nil {
					return err
				}
				if done {
					withdrawn++
				}
			}
			return nil
		})
		if err != nil || len(page) < evidenceAuthorshipBatch {
			return withdrawn, err
		}
	}
}

// meetingStillProvesHeld is MeetingWasHeld over the meeting as stored now,
// linked or not. found is false when the meeting no longer exists.
func meetingStillProvesHeld(ctx context.Context, tx pgx.Tx, activityID ids.UUID, own OwnDomainReader) (bool, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, markerPos := arg(activityID), arg(TranscriptSourceSystem)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return false, false, err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	var status *string
	var transcript bool
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.meeting_status, a.source_system IS NOT DISTINCT FROM $%[2]d AND coalesce(a.body, '') <> ''
		  FROM activity a WHERE a.id = $%[1]d%[3]s`, idPos, markerPos, scope), args...).Scan(&status, &transcript)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("deals: reading a meeting to judge again: %w", err)
	}
	participants, err := readActivityParticipants(ctx, tx, activityID)
	if err != nil {
		return false, false, err
	}
	parties, err := own.MeetingParties(ctx, tx)
	if err != nil {
		return false, false, err
	}
	var meetingStatus string
	if status != nil {
		meetingStatus = *status
	}
	return MeetingWasHeld(meetingStatus, transcript, BuyerParticipantCount(participants, parties)).Held, true, nil
}

// withdrawMeetingClaim marks a machine meeting claim not met, with the audit
// row and the deal event every evidence change carries. It writes nothing
// when the row changed or was refuted since the page was read.
func withdrawMeetingClaim(ctx context.Context, tx pgx.Tx, claim meetingClaim) (bool, error) {
	lock, err := storekit.LockRow(ctx, tx, evidenceEntity, claim.ID, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var met, refuted bool
	if err := tx.QueryRow(ctx, `SELECT met, refuted_at IS NOT NULL FROM deal_stage_evidence WHERE id = $1`, claim.ID).
		Scan(&met, &refuted); err != nil {
		return false, fmt.Errorf("deals: rereading a meeting claim under its lock: %w", err)
	}
	if !met || refuted {
		return false, nil
	}
	patch := storekit.NewPatch()
	patch.Set("met", true, false)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("deals: withdrawing a meeting claim: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", evidenceEntity, claim.ID,
		patch.Before(), patch.After(), map[string]any{"rejudged_against": "meeting_parties"})
	if err != nil {
		return false, fmt.Errorf("deals: auditing a withdrawn meeting claim: %w", err)
	}
	return true, emitEvidenceChanged(ctx, tx, auditID, ids.From[ids.DealKind](claim.DealID))
}
