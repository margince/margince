// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a meeting-copy fold handed the survivor, read back so a repair that
// splits a wrong fold can take that back.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MeetingCopySurvivor answers the row a meeting copy folded into, as its
// archive evidence names it, or false for any other archive.
func MeetingCopySurvivor(evidence []byte) (ids.ActivityID, bool) {
	var recorded map[string]any
	if json.Unmarshal(evidence, &recorded) != nil || recorded[evidenceReasonKey] != meetingCopyReason {
		return ids.ActivityID{}, false
	}
	named, ok := recorded[meetingCopySurvivorKey].(string)
	if !ok {
		return ids.ActivityID{}, false
	}
	survivor, err := ids.Parse(named)
	if err != nil {
		return ids.ActivityID{}, false
	}
	return ids.From[ids.ActivityKind](survivor), true
}

// MeetingCopyHandedOnDocumentsTx reports whether folding copy into survivor
// moved document links onto the survivor.
func MeetingCopyHandedOnDocumentsTx(ctx context.Context, tx pgx.Tx, copyID, survivor ids.ActivityID) (bool, error) {
	var handedOn bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log
		                WHERE entity_type = 'activity' AND entity_id = $1 AND evidence ->> $3 = $2::uuid::text)`,
		survivor, copyID, meetingCopyFoldedFromKey).Scan(&handedOn)
	if err != nil {
		return false, fmt.Errorf("activities: reading whether a meeting copy handed on documents: %w", err)
	}
	return handedOn, nil
}

// MeetingCopyCarriedGuestsTx answers the guest rows folding copyID into
// survivor added to the survivor, and false when the fold's record cannot
// tell. A fold lists them in its archive evidence. An older fold listed none,
// so its rows are the survivor's guests that the fold's own transaction
// inserted. They count only when that transaction folded just this one copy
// onto the survivor and each row repeats a guest of the copy.
func MeetingCopyCarriedGuestsTx(
	ctx context.Context, tx pgx.Tx, copyID, survivor ids.ActivityID, evidence []byte, foldedAt time.Time,
) ([]ids.UUID, bool, error) {
	listed, recorded, readable := carriedGuestsListed(evidence)
	if !readable || recorded {
		return listed, readable, nil
	}
	var folds int
	var carried []ids.UUID
	var copiesAll bool
	err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM audit_log a
		         WHERE a.entity_type = 'activity' AND a.action = 'archive' AND a.occurred_at = $3
		           AND a.evidence ->> $4 = $2::uuid::text),
		       coalesce(array_agg(s.id) FILTER (WHERE s.id IS NOT NULL), '{}'),
		       coalesce(bool_and(EXISTS (SELECT 1 FROM activity_participant d WHERE d.activity_id = $1
		                 AND (d.user_id, d.contact_id, d.address, d.channel_user_id, d.role)
		                     IS NOT DISTINCT FROM (s.user_id, s.contact_id, s.address, s.channel_user_id, s.role)))
		                FILTER (WHERE s.id IS NOT NULL), true)
		  FROM (SELECT NULL::uuid AS id) none
		  LEFT JOIN activity_participant s ON s.activity_id = $2 AND s.created_at = $3`,
		copyID, survivor, foldedAt, meetingCopySurvivorKey).Scan(&folds, &carried, &copiesAll)
	if err != nil {
		return nil, false, fmt.Errorf("activities: reading the guests a meeting copy handed on: %w", err)
	}
	return carried, folds == 1 && copiesAll, nil
}

// UndoMeetingCopyGuestsTx removes the guest rows a fold carried onto survivor.
// It is audited on the survivor with the copy they came from, as the fold's
// own hand-over of documents is.
func UndoMeetingCopyGuestsTx(ctx context.Context, tx pgx.Tx, copyID, survivor ids.ActivityID, guests []ids.UUID) error {
	if len(guests) == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `DELETE FROM activity_participant WHERE activity_id = $1 AND id = ANY($2)`, survivor, guests)
	if err != nil {
		return fmt.Errorf("activities: removing the guests a meeting copy handed on: %w", err)
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", survivor.UUID,
		map[string]any{"meeting_guests_removed": tag.RowsAffected()}, map[string]any{"split_from": copyID.String()})
	if err != nil {
		return fmt.Errorf("activities: auditing the guests a split took back: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, survivor.UUID, crmcontracts.PublicEventActivityUpdated{})
}

// carriedGuestsListed reads the guest rows a fold listed in its archive
// evidence. recorded is false for a fold that kept no list. Evidence that does
// not parse is not readable, and the caller then refuses rather than guess.
func carriedGuestsListed(evidence []byte) (carried []ids.UUID, recorded, readable bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(evidence, &fields) != nil {
		return nil, false, false
	}
	listed, recorded := fields[meetingCopyCarriedGuestsKey]
	if !recorded {
		return nil, false, true
	}
	readable = json.Unmarshal(listed, &carried) == nil
	return carried, true, readable
}
