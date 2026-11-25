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

// The audit images of a lowercased activity count its participant rows, and
// those still to lowercase. They hold no address.
const (
	auditFieldParticipantRows    = "participant_rows"
	auditFieldAddressesAsWritten = "participant_addresses_not_lowercase"
)

// participantSpellings counts an activity's participant rows, and those whose
// address is not lowercase.
type participantSpellings struct {
	Activity         ids.UUID
	Rows, NotLowered int
}

// LowercaseParticipantAddressesTx lowercases the address of up to limit older
// participant rows after the cursor. It returns the rows it inspected and how
// many it changed, and audits each activity as an update.
func LowercaseParticipantAddressesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, activity_id `+addressRowsToLowercase+` AND id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant addresses to lowercase: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ ID, Activity ids.UUID }])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading participant addresses to lowercase: %w", err)
	}
	if len(page) == 0 {
		return nil, 0, nil
	}
	inspected, activities := make([]ids.UUID, len(page)), make([]ids.UUID, len(page))
	for i, row := range page {
		inspected[i], activities[i] = row.ID, row.Activity
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activities...); err != nil {
		return nil, 0, err
	}
	// Every row of the batch's activities is locked. The two counts below are
	// then of one set of rows, and differ only by what this batch folded.
	rows, err = tx.Query(ctx, `SELECT id FROM activity_participant WHERE activity_id = ANY($1) ORDER BY id FOR UPDATE`, activities)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: locking the participant rows of activities to lowercase: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: locking the participant rows of activities to lowercase: %w", err)
	}
	before, err := countParticipantSpellings(ctx, tx, held)
	if err != nil {
		return nil, 0, err
	}
	changed, err := storekit.LowercaseOlderParticipantAddresses(ctx, tx, `ap.id = ANY($1)`, inspected)
	if err != nil {
		return nil, 0, err
	}
	lowered, err := countParticipantSpellings(ctx, tx, held)
	if err != nil {
		return nil, 0, err
	}
	left := make(map[ids.UUID]participantSpellings, len(lowered))
	for _, activity := range lowered {
		left[activity.Activity] = activity
	}
	for _, activity := range before {
		if err := auditLowercasedParticipants(ctx, tx, activity, left[activity.Activity], runID); err != nil {
			return nil, 0, err
		}
	}
	return inspected, changed, nil
}

// countParticipantSpellings counts, per activity and in activity order, those
// of the given participant rows that are stored.
func countParticipantSpellings(ctx context.Context, tx pgx.Tx, participants []ids.UUID) ([]participantSpellings, error) {
	rows, err := tx.Query(ctx, `
		SELECT activity_id, count(*), count(*) FILTER (WHERE address <> lower(btrim(address)))
		  FROM activity_participant WHERE id = ANY($1) GROUP BY activity_id ORDER BY activity_id`, participants)
	if err != nil {
		return nil, fmt.Errorf("activities: counting the participant rows of activities to lowercase: %w", err)
	}
	counts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[participantSpellings])
	if err != nil {
		return nil, fmt.Errorf("activities: counting the participant rows of activities to lowercase: %w", err)
	}
	return counts, nil
}

// auditLowercasedParticipants records what the correction did to one activity
// and announces it, as a roster change announces the activity.
func auditLowercasedParticipants(ctx context.Context, tx pgx.Tx, before, after participantSpellings, runID ids.UUID) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", before.Activity,
		map[string]any{auditFieldParticipantRows: before.Rows, auditFieldAddressesAsWritten: before.NotLowered},
		map[string]any{auditFieldParticipantRows: after.Rows, auditFieldAddressesAsWritten: after.NotLowered},
		map[string]any{evidenceDataTaskRun: runID.String()})
	if err != nil {
		return err
	}
	// The interaction graph refolds an activity's participants on this event.
	return storekit.EmitEvent(ctx, tx, auditID, before.Activity, crmcontracts.PublicEventActivityUpdated{})
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
