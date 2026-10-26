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
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingProofPage selects unrefuted machine claims that a meeting was held,
// met or not. Only this pass writes one not met, so it may restore it. A
// model's reading rests on the meeting's text, so it is not judged here.
const meetingProofPage = `
	SELECT ev.id, ev.deal_id, ev.source_id, ev.met
	  FROM deal_stage_evidence ev
	  JOIN stage_exit_criterion c ON c.id = ev.criterion_id
	 WHERE ev.source_type = $1 AND ev.extracted_by = $2 AND c.kind = $3
	   AND ev.refuted_at IS NULL AND ev.id > $4
	 ORDER BY ev.id LIMIT $5`

type meetingClaim struct {
	ID       ids.UUID
	DealID   ids.UUID
	SourceID ids.UUID
	Met      bool
}

// reconcileMeetingProof judges again whether each meeting behind a machine
// event_held claim proves it was held, by MeetingWasHeld, and sets the claim
// met to match. It answers how many claims it changed.
func (s *Store) reconcileMeetingProof(ctx context.Context, own OwnDomainReader) (int, error) {
	changed := 0
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
			n, err := settleMeetingClaims(ctx, tx, page, own)
			changed += n
			return err
		})
		if err != nil || len(page) < evidenceAuthorshipBatch {
			return changed, err
		}
		after = page[len(page)-1].ID
	}
}

// settleMeetingClaims judges each meeting once, under its row lock, and sets
// every claim on it to what the meeting proves now.
func settleMeetingClaims(ctx context.Context, tx pgx.Tx, page []meetingClaim, own OwnDomainReader) (int, error) {
	if len(page) == 0 {
		return 0, nil
	}
	parties, err := own.MeetingParties(ctx, tx)
	if err != nil {
		return 0, err
	}
	type verdict struct{ held, found bool }
	judged := map[ids.UUID]verdict{}
	changed := 0
	for _, claim := range page {
		v, known := judged[claim.SourceID]
		if !known {
			held, found, err := meetingProvesHeld(ctx, tx, claim.SourceID, parties)
			if err != nil {
				return changed, err
			}
			v = verdict{held, found}
			judged[claim.SourceID] = v
		}
		if !v.found || v.held == claim.Met {
			continue
		}
		done, err := setMeetingClaimMet(ctx, tx, claim, v.held)
		if err != nil {
			return changed, err
		}
		if done {
			changed++
		}
	}
	return changed, nil
}

// meetingStillProvesHeld is meetingProvesHeld with the own domains read now.
func meetingStillProvesHeld(ctx context.Context, tx pgx.Tx, activityID ids.UUID, own OwnDomainReader) (bool, bool, error) {
	parties, err := own.MeetingParties(ctx, tx)
	if err != nil {
		return false, false, err
	}
	return meetingProvesHeld(ctx, tx, activityID, parties)
}

// meetingProvesHeld is MeetingWasHeld over the meeting as stored now, linked
// or not. found is false when the meeting no longer exists. The share lock
// holds a status or attendee change off until the caller's write commits.
func meetingProvesHeld(ctx context.Context, tx pgx.Tx, activityID ids.UUID, parties owndomain.Parties) (bool, bool, error) {
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
		  FROM activity a WHERE a.id = $%[1]d%[3]s FOR SHARE OF a`, idPos, markerPos, scope), args...).Scan(&status, &transcript)
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
	var meetingStatus string
	if status != nil {
		meetingStatus = *status
	}
	return MeetingWasHeld(meetingStatus, transcript, BuyerParticipantCount(participants, parties)).Held, true, nil
}

// setMeetingClaimMet sets a machine meeting claim met or not, with the audit
// row and the deal event every evidence change carries. It writes nothing
// when the row changed or was refuted since the page was read.
func setMeetingClaimMet(ctx context.Context, tx pgx.Tx, claim meetingClaim, met bool) (bool, error) {
	lock, err := storekit.LockRow(ctx, tx, evidenceEntity, claim.ID, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var current, refuted bool
	if err := tx.QueryRow(ctx, `SELECT met, refuted_at IS NOT NULL FROM deal_stage_evidence WHERE id = $1`, claim.ID).
		Scan(&current, &refuted); err != nil {
		return false, fmt.Errorf("deals: rereading a meeting claim under its lock: %w", err)
	}
	if current != claim.Met || refuted {
		return false, nil
	}
	patch := storekit.NewPatch()
	patch.Set("met", current, met)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("deals: setting a meeting claim: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", evidenceEntity, claim.ID,
		patch.Before(), patch.After(), map[string]any{"rejudged_against": "meeting_parties"})
	if err != nil {
		return false, fmt.Errorf("deals: auditing a meeting claim: %w", err)
	}
	return true, emitEvidenceChanged(ctx, tx, auditID, ids.From[ids.DealKind](claim.DealID))
}
