// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An activity row as the activities store's inputs. The cell check has already
// read the type, timestamp, duration and links into typed; this only places them.

import (
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The columns of activities.csv beside ref, author and owner_email.
const (
	activityTargetType          = "type"
	activityTargetSubject       = "subject"
	activityTargetBody          = "body"
	activityTargetOccurredAt    = "occurred_at"
	activityTargetLinks         = "links"
	activityTargetDuration      = "duration_minutes"
	activityTargetMeetingStatus = "meeting_status"
	activityTargetDirection     = "direction"
)

// activityTypedPrecision is the typed key the cell check sets when the
// timestamp cell named a day without a time. It is no column of the file.
const activityTypedPrecision = "occurred_precision"

// activityStoredColumns names the stored activity's JSON field for each target
// the file spells differently. The duration is compared in seconds, the unit
// the cell check typed it in.
//
//nolint:goconst // the activity wire's own keys; another surface's constant of one spelling is another vocabulary
var activityStoredColumns = map[string]string{
	activityTargetType:     "kind",
	activityTargetDuration: "duration_seconds",
}

// activityCreateFrom builds the create input for one checked activity row.
// The idempotency key stays empty: the identity map is the one authority on
// which row landed, and a source key would replay an archived row.
func activityCreateFrom(fields map[string]string, typed map[string]any, sourceSystem, source string) activities.LogActivityInput {
	in := activities.LogActivityInput{
		SourceSystem:    &sourceSystem,
		Source:          source,
		Subject:         importString(fields, activityTargetSubject),
		Body:            importString(fields, activityTargetBody),
		OccurredAt:      activityOccurredAt(fields, typed),
		DurationSeconds: activityDuration(fields, typed),
		Author:          authorFrom(fields),
	}
	in.OccurredPrecision, _ = typed[activityTypedPrecision].(string)
	// The owner is who the activity is attributed to. An address no seat holds
	// leaves the importer as the author.
	if seat, ok := typed[csvTargetOwner].(ids.UUID); ok {
		in.Author.AuthorID = &seat
	}
	in.Kind, _ = typed[activityTargetType].(string)
	if direction, ok := typed[activityTargetDirection].(string); ok {
		in.Direction = &direction
	}
	if status, ok := typed[activityTargetMeetingStatus].(string); ok {
		in.MeetingStatus = &status
	}
	in.Links, _ = typed[activityTargetLinks].([]activities.ActivityLinkInput)
	return in
}

// activityUpdateFrom builds the update for the cells that differ from the stored
// activity. The kind, direction, links and author are fixed when it lands.
func activityUpdateFrom(changed map[string]string, typed map[string]any) activities.UpdateActivityInput {
	in := activities.UpdateActivityInput{
		Subject:         importString(changed, activityTargetSubject),
		Body:            importString(changed, activityTargetBody),
		OccurredAt:      activityOccurredAt(changed, typed),
		DurationSeconds: activityDuration(changed, typed),
	}
	// A new time alone is an instant to the store, so a day cell says so.
	if precision, ok := typed[activityTypedPrecision].(string); ok && in.OccurredAt != nil {
		in.OccurredPrecision = &precision
	}
	if _, given := changed[activityTargetMeetingStatus]; given {
		if status, ok := typed[activityTargetMeetingStatus].(string); ok {
			in.MeetingStatus = &status
		}
	}
	return in
}

// activityOccurredAt is the checked timestamp, nil when the row maps none.
func activityOccurredAt(fields map[string]string, typed map[string]any) *time.Time {
	if _, given := fields[activityTargetOccurredAt]; !given {
		return nil
	}
	if at, ok := typed[activityTargetOccurredAt].(time.Time); ok {
		return &at
	}
	return nil
}

// activityDuration is the checked duration in seconds, nil when the row maps none.
func activityDuration(fields map[string]string, typed map[string]any) *int {
	if _, given := fields[activityTargetDuration]; !given {
		return nil
	}
	if seconds, ok := typed[activityTargetDuration].(int); ok {
		return &seconds
	}
	return nil
}

// storedInstant is a stored timestamp in UTC, the zone the cell check reads a
// file's timestamp into. The store hands it back in the server's zone, which
// would otherwise differ from an unedited cell in its spelling alone.
func storedInstant(raw json.RawMessage) json.RawMessage {
	var at time.Time
	if err := json.Unmarshal(raw, &at); err != nil {
		return raw
	}
	utc, err := json.Marshal(at.UTC())
	if err != nil {
		return raw
	}
	return utc
}
