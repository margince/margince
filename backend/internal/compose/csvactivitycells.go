// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An activity row's own cells, read the way the activities store takes them.
// The type is a kind, the timestamp carries its zone, the duration is in seconds.
// The kind rules are the store's own (activities.CheckKindFields).

import (
	"slices"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// importableKinds are the types a file may name. A message needs a channel and
// a task would land open on the importer's list, so neither is imported.
var importableKinds = []string{
	string(crmcontracts.ActivityKindNote), string(crmcontracts.ActivityKindCall),
	string(crmcontracts.ActivityKindMeeting), string(crmcontracts.ActivityKindEmail),
}

var meetingStatuses = []string{
	string(crmcontracts.ActivityMeetingStatusBooked), string(crmcontracts.ActivityMeetingStatusHeld),
	string(crmcontracts.ActivityMeetingStatusNoShow), string(crmcontracts.ActivityMeetingStatusCanceled),
}

var directions = []string{string(crmcontracts.ActivityDirectionInbound), string(crmcontracts.ActivityDirectionOutbound)}

// maxDurationMinutes is the first minute count past what the store accepts.
// A larger cell is refused before it is multiplied into seconds.
const maxDurationMinutes = 31*24*60 + 1

// zonelessLayouts are timestamps that carry a time and no zone.
var zonelessLayouts = []string{"2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02T15:04:05"}

// zonedLayouts are timestamps that carry their zone. A day alone is read apart.
var zonedLayouts = []string{time.RFC3339, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04Z07:00"}

// checkActivityCells reads an activity row's type, timestamp, duration, meeting
// status and direction into typed. The first problem wins.
func checkActivityCells(fields map[string]string, typed map[string]any, now time.Time) (map[string]any, *cellProblem) {
	if typed == nil {
		typed = map[string]any{}
	}
	steps := []func(map[string]string, map[string]any, time.Time) *cellProblem{
		checkActivityType, checkOccurredAt, checkDuration, checkMeetingStatus, checkDirection, checkKindFields,
	}
	for _, step := range steps {
		if problem := step(fields, typed, now); problem != nil {
			return nil, problem
		}
	}
	return typed, nil
}

func checkActivityType(fields map[string]string, typed map[string]any, _ time.Time) *cellProblem {
	raw, mapped := fields[activityTargetType]
	if !mapped {
		return nil
	}
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == "":
		return problemOf(activityTargetType, raw, migration.Why(migration.ReasonActivityTypeUnnamed))
	case text == string(crmcontracts.ActivityKindTask):
		return problemOf(activityTargetType, raw, migration.Why(migration.ReasonTaskNotImportable))
	case text != string(crmcontracts.ActivityKindMessage) && !slices.Contains(importableKinds, text):
		return problemOf(activityTargetType, raw, migration.Why(migration.ReasonActivityTypeUnknown,
			"value", strings.TrimSpace(raw), "allowed", strings.Join(importableKinds, ", ")))
	}
	// A message passes here and is refused by the store's own rule in
	// checkKindFields: a file row has no transport to name.
	typed[activityTargetType] = text
	return nil
}

// checkOccurredAt reads the timestamp: a day is midnight UTC. Any other instant
// must carry its zone, because a time read in the server's zone is a time
// nobody wrote. It converts to UTC so an unedited cell compares equal.
func checkOccurredAt(fields map[string]string, typed map[string]any, now time.Time) *cellProblem {
	raw, mapped := fields[activityTargetOccurredAt]
	if !mapped {
		return nil
	}
	text := strings.TrimSpace(raw)
	if text == "" {
		return problemOf(activityTargetOccurredAt, raw, migration.Why(migration.ReasonOccurredAtUnnamed))
	}
	at, problem := parseTimestamp(text)
	if problem != nil {
		return problemOf(activityTargetOccurredAt, raw, *problem)
	}
	if at.After(now) {
		return problemOf(activityTargetOccurredAt, raw, migration.Why(migration.ReasonOccurredAtFuture, "value", text))
	}
	typed[activityTargetOccurredAt] = at
	return nil
}

// parseTimestamp reads a day or a zoned instant as UTC, or says why it cannot.
func parseTimestamp(text string) (time.Time, *migration.Reason) {
	if day, err := time.Parse(time.DateOnly, text); err == nil {
		return day, nil
	}
	for _, layout := range zonedLayouts {
		if at, err := time.Parse(layout, text); err == nil {
			return at.UTC(), nil
		}
	}
	for _, layout := range zonelessLayouts {
		if _, err := time.Parse(layout, text); err == nil {
			why := migration.Why(migration.ReasonTimestampNeedsZone, "value", text)
			return time.Time{}, &why
		}
	}
	why := migration.Why(migration.ReasonNotADate, "value", text)
	return time.Time{}, &why
}

// checkDuration reads whole minutes and types them as the seconds the store holds.
func checkDuration(fields map[string]string, typed map[string]any, _ time.Time) *cellProblem {
	raw, mapped := fields[activityTargetDuration]
	text := strings.TrimSpace(raw)
	if !mapped || text == "" {
		return nil
	}
	minutes, err := strconv.Atoi(text)
	if err != nil {
		return problemOf(activityTargetDuration, raw, migration.Why(migration.ReasonNotANumber, "value", text))
	}
	if minutes < 0 || minutes >= maxDurationMinutes {
		return problemOf(activityTargetDuration, raw, migration.Why(migration.ReasonDurationOutOfRange, "value", text))
	}
	typed[activityTargetDuration] = minutes * 60
	return nil
}

func checkMeetingStatus(fields map[string]string, typed map[string]any, _ time.Time) *cellProblem {
	return checkWord(fields, typed, activityTargetMeetingStatus, meetingStatuses, migration.ReasonMeetingStatusUnknown)
}

func checkDirection(fields map[string]string, typed map[string]any, _ time.Time) *cellProblem {
	return checkWord(fields, typed, activityTargetDirection, directions, migration.ReasonDirectionUnknown)
}

// checkWord reads a cell that must be one of a closed vocabulary, in any case.
// An empty cell says nothing.
func checkWord(fields map[string]string, typed map[string]any, target string, allowed []string, unknown migration.ReasonCode) *cellProblem {
	raw := fields[target]
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return nil
	}
	if !slices.Contains(allowed, text) {
		return problemOf(target, raw, migration.Why(unknown, "value", strings.TrimSpace(raw), "allowed", strings.Join(allowed, ", ")))
	}
	typed[target] = text
	return nil
}

// checkKindFields holds the row's duration and meeting status against its kind
// by the store's own rule. A row naming no type is held to the stored kind later.
func checkKindFields(fields map[string]string, typed map[string]any, _ time.Time) *cellProblem {
	kind, _ := typed[activityTargetType].(string)
	if kind == "" {
		return nil
	}
	var seconds *int
	if s, ok := typed[activityTargetDuration].(int); ok {
		seconds = &s
	}
	var status *string
	if s, ok := typed[activityTargetMeetingStatus].(string); ok {
		status = &s
	}
	err := activities.CheckKindFields(kind, seconds, status, "")
	if err == nil {
		return nil
	}
	target, why, known := activityKindReason(err, fields)
	if !known {
		return nil
	}
	return problemOf(target, fields[target], why)
}

// activityKindReason says in the report's words why the store's kind rules
// refused an activity, and which column to correct. It is false for an error
// that is not one of those rules.
func activityKindReason(err error, fields map[string]string) (string, migration.Reason, bool) {
	switch {
	case wrapsA[*activities.MessageProviderError](err):
		return activityTargetType, migration.Why(migration.ReasonMessageNotImportable), true
	case wrapsA[*activities.MeetingStatusKindError](err):
		return activityTargetMeetingStatus, migration.Why(migration.ReasonMeetingStatusNotForKind), true
	case wrapsA[*activities.KindFieldError](err):
		return activityTargetDuration, migration.Why(migration.ReasonDurationNotValidForKind), true
	case wrapsA[*activities.SchedulingArgumentError](err):
		return activityTargetDuration, migration.Why(migration.ReasonDurationOutOfRange,
			"value", strings.TrimSpace(fields[activityTargetDuration])), true
	}
	return "", migration.Reason{}, false
}

// nowUTC is the writer's clock, which a test sets and a run leaves to the system.
func (w *csvWriters) nowUTC() time.Time {
	if w.now != nil {
		return w.now().UTC()
	}
	return time.Now().UTC()
}
