// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addressRowsToLowercase selects the participant rows an older writer stored
// with the address as typed. idx_aparticipant_address_to_lowercase holds them.
const addressRowsToLowercase = `FROM activity_participant WHERE address <> lower(btrim(address))`

// auditFieldAddressesAsWritten counts, in the audit images of a correction, the
// addresses of one batch on an activity that are not lowercase. No address is held.
const auditFieldAddressesAsWritten = "participant_addresses_not_lowercase"

// LowercaseParticipantAddressesTx lowercases the address of up to limit older
// participant rows after the cursor. It returns the rows it inspected and how
// many it changed, and audits each activity as an update.
func LowercaseParticipantAddressesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	var inspected, activities []ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(page.id ORDER BY page.id), '{}'), coalesce(array_agg(page.activity_id ORDER BY page.id), '{}')
		  FROM (SELECT id, activity_id `+addressRowsToLowercase+` AND id > $1 ORDER BY id LIMIT $2) page`,
		after, limit).Scan(&inspected, &activities); err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant addresses to lowercase: %w", err)
	}
	if len(inspected) == 0 {
		return nil, 0, nil
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activities...); err != nil {
		return nil, 0, err
	}
	changed, err := storekit.LowercaseOlderParticipantAddresses(ctx, tx, `ap.id = ANY($1)`, inspected)
	if err != nil {
		return nil, 0, err
	}
	lowered := map[ids.UUID]int{}
	for _, activity := range activities {
		lowered[activity]++
	}
	for activity, addresses := range lowered {
		if err := auditLowercasedParticipants(ctx, tx, activity, addresses, runID); err != nil {
			return nil, 0, err
		}
	}
	return inspected, changed, nil
}

// auditLowercasedParticipants records the correction on one activity and
// announces it, as a roster change announces the activity.
func auditLowercasedParticipants(ctx context.Context, tx pgx.Tx, activity ids.UUID, addresses int, runID ids.UUID) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activity,
		map[string]any{auditFieldAddressesAsWritten: addresses}, map[string]any{auditFieldAddressesAsWritten: 0},
		map[string]any{evidenceDataTaskRun: runID.String()})
	if err != nil {
		return err
	}
	// The interaction graph refolds an activity's participants on this event.
	return storekit.EmitEvent(ctx, tx, auditID, activity, crmcontracts.PublicEventActivityUpdated{})
}

// CountParticipantAddressesToLowercase counts the rows
// LowercaseParticipantAddressesTx has yet to change, reading at most maximum+1.
func CountParticipantAddressesToLowercase(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+addressRowsToLowercase+` LIMIT $1) as_written`, maximum+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("activities: counting participant addresses to lowercase: %w", err)
	}
	return count, nil
}
