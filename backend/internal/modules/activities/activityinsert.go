// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func insertLoggedActivityRow(ctx context.Context, tx pgx.Tx, in LogActivityInput, by string, assignee, host *ids.UserID) (ids.ActivityID, time.Time, error) {
	id := ids.New[ids.ActivityKind]()
	origin := in.Origin
	if origin == "" {
		origin = OriginHuman
	}
	counterparty, entity, err := prepareLoggedCorrespondence(ctx, tx, in, host)
	if err != nil {
		return ids.ActivityID{}, time.Time{}, err
	}
	if in.ThreadKey == "" && in.ownThread {
		in.ThreadKey = ownThreadKey(id.UUID)
	}
	authorCols, authorHolders, args := storekit.AuthorInsertFragments(in.Author, []any{
		// NULLIF on channel_provider: the column FKs into channel_provider, and
		// '' names no provider, so anything without a transport stores NULL.
		id, in.Kind, in.ChannelProvider, in.Subject, in.Body, in.OccurredAt, in.Direction, in.MeetingStatus,
		in.DueAt, in.RemindAt, assignee, host, in.ClaimsHostSlot, in.SourceSystem, in.SourceID, in.Source, by,
		in.ThreadKey, counterparty, in.CounterpartyOutboundAttested, origin,
		in.SourceActivityID, in.Raw, in.DurationSeconds, loggedSenderIsMachine(in), in.DueHasTime, in.CallOutcome,
		in.TaskType,
	})
	args = append(args, entity)
	entityHolder := storekit.SQLf("$%d", len(args))
	systemCol, systemHolder, args := systemTextInsert(ctx, in, args)
	precisionCol, precisionHolder, args := precisionInsertFragment(in.OccurredPrecision, args)
	languageCol, languageHolder, args := languageInsertFragment(in, args)
	cfCols, cfHolders, args, err := storekit.InsertFragments(in.customColumns, in.CustomFields, args)
	if err != nil {
		return ids.ActivityID{}, time.Time{}, err
	}
	// An undated activity takes the transaction's now(): the last-activity clock
	// counts only occurred_at <= now(), and a Go clock read here runs ahead of it.
	var occurredAt time.Time
	err = tx.QueryRow(ctx,
		`INSERT INTO activity (id, kind, channel_provider, subject, body, occurred_at, direction, meeting_status,
		                       due_at, remind_at, assignee_id, host_user_id, claims_host_slot, booking_interval_exact, source_system, source_id, source, captured_by,
		                       thread_key, counterparty_email, counterparty_outbound_attested, origin,
		                       source_activity_id, raw, duration_seconds, sender_is_machine, due_has_time, call_outcome, task_type`+authorCols+`, legal_entity_id`+precisionCol+languageCol+systemCol+cfCols+`)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5, coalesce($6, now()), $7, $8, $9, $10, $11, $12, $13, $13, $14, $15, $16, $17, NULLIF($18, ''),
		         NULLIF($19, ''), $20, $21, $22, $23, $24, $25, $26, $27, $28`+authorHolders+`, `+entityHolder+precisionHolder+languageHolder+systemHolder+cfHolders+`) RETURNING occurred_at`,
		args...).Scan(&occurredAt)
	if err != nil {
		if storekit.IsUniqueViolation(err) {
			return ids.ActivityID{}, time.Time{}, apperrors.ErrConflict
		}
		return ids.ActivityID{}, time.Time{}, err
	}

	return id, occurredAt, nil
}

// Satellites and the first meeting transition commit with the activity row.
func writeActivitySatellites(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, in LogActivityInput,
	occurredAt time.Time, by string,
) error {
	if err := insertActivityLinks(ctx, tx, id, in.Kind, in.Links, in.LinksFromRequest); err != nil {
		return err
	}
	// The first transition, where this capture named a status. A meeting
	// arrives `booked` far more often than not, and that booking is the fact
	// every "how many did we book this period" question counts.
	if err := recordMeetingTransition(ctx, tx, meetingTransition{
		RecordedAt:   in.recordedAt,
		ActivityID:   id,
		Status:       meetingStatusOrNone(in.MeetingStatus),
		Scheduled:    scheduledAs(occurredAt, storedPrecision(in.OccurredPrecision)),
		SourceSystem: in.SourceSystem,
		SourceID:     in.SourceID,
	}); err != nil {
		return err
	}
	// Link admission already checked the participants' parent visibility.
	if err := stampLoggedParticipants(ctx, tx, id, in); err != nil {
		return err
	}
	if err := recordImportedProvenance(ctx, tx, id, in, by); err != nil {
		return err
	}
	return recordInitialActivity(ctx, tx, id, in)
}

func logActivityInTx(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if err := prepareLoggedActivity(ctx, tx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	assignee, err := checkedTaskAssignee(ctx, tx, in)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	host := meetingHost(ctx, in)

	if replayed, found, err := replayLanding(ctx, tx, in); err != nil || found {
		return replayed, false, err
	}
	if prior, found, err := scheduledRunNoteLanding(ctx, tx, &in); err != nil || found {
		return prior, false, err
	}
	if bound, found, err := recognizedMessage(ctx, tx, in); err != nil || found {
		return bound, false, err
	}
	if err := refuseLoggedOutcomeBeforeStart(ctx, tx, in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := applyLoggedDealFiling(ctx, tx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}

	id, occurredAt, err := insertLoggedActivityRow(ctx, tx, in, by, assignee, host)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}

	if err := writeActivitySatellites(ctx, tx, id, in, occurredAt.UTC(), by); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	out, err := readActivityAt(ctx, tx, id, storekit.LiveOnly, orWallClock(in.recordedAt))
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return out, true, nil
}
