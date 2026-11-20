// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// unchangedEdgeGuard turns the upsert into a no-op for a row whose folded
// facts are already what the fold produced, so computed_at and the tuple stay
// as they were.
//
// read_snapshot is the one reader of a rewrite: uncoveredGraphSourceSQL asks
// whether an edge's snapshot saw a history import's source transaction, and
// answers "not covered" for an older one. A kept snapshot can only make that
// answer more cautious, which costs one more fold and never a stale edge. The
// folds that exist to cover a source (the queue's prepare step and the drain)
// pass advanceSnapshot and always write, so coverage is still established
// there. Nothing else reads computed_at.
func unchangedEdgeGuard(advanceSnapshot bool, facts []string) string {
	if advanceSnapshot {
		return ""
	}
	existing := make([]string, len(facts))
	excluded := make([]string, len(facts))
	for i, fact := range facts {
		existing[i] = "e." + fact
		excluded[i] = "EXCLUDED." + fact
	}
	return "\n\t\t WHERE (" + strings.Join(existing, ", ") + ") IS DISTINCT FROM (" + strings.Join(excluded, ", ") + ")"
}

// participantRows stands for the activity_participant rows, which every fold
// reads and which no activity column describes.
const participantRows = "participant rows"

// activityColumnRef finds `a.<column>` and `"a".<column>`, the two spellings
// the fold's SQL fragments use for the activity alias.
var activityColumnRef = regexp.MustCompile(`(?:^|[^\w."])"?a"?\.([a-z_]+)`)

// foldReadsActivityColumns is every activity column the edge folds name in
// their SQL, taken from the statements themselves so a column a fold starts
// to read is covered without anyone editing a list.
var foldReadsActivityColumns = columnsNamedBy(
	foldPairsSQL(false), pruneInteractionEdgesSQL(),
	foldContactPairsSQL(false), pruneContactEdgesSQL(), countingActivitiesSQL,
)

func columnsNamedBy(statements ...string) map[string]bool {
	columns := map[string]bool{participantRows: true}
	for _, statement := range statements {
		for _, match := range activityColumnRef.FindAllStringSubmatch(statement, -1) {
			columns[match[1]] = true
		}
	}
	return columns
}

// Column names the map below repeats for the keys that share one write.
const (
	colDueAt          = "due_at"
	colCallOutcome    = "call_outcome"
	colDirection      = "direction"
	colMeetingStatus  = "meeting_status"
	colRemindAt       = "remind_at"
	colSourceActivity = "source_activity_id"
)

// updatedFieldWrites says which activity columns each key of an
// activity.updated event's changed_fields is written from. An empty list is a
// key whose write lands outside every column the folds read. A key missing
// from this map is unknown, and unknown refolds.
// A test holds this map to the event's schema, so a new key must be placed here.
var updatedFieldWrites = map[string][]string{
	"agenda_changed":                {},
	"assignee_id":                   {"assignee_id"},
	"audience":                      {"audience", "internal_mail_authorization"},
	"automatic_task_repaired":       {colSourceActivity, colDueAt},
	"body":                          {"body"},
	"calendar_method":               {"calendar_method"},
	"call_outcome":                  {colCallOutcome},
	"call_outcome_cleared":          {colCallOutcome},
	"commitment_admissions_changed": {},
	"direction":                     {colDirection, "counterparty_email"},
	"due_at":                        {colDueAt},
	"due_has_time":                  {"due_has_time"},
	"handling_automation_id":        {"handling_automation_id"},
	"host_user_id":                  {participantRows},
	"is_done":                       {"is_done"},
	"meeting_recap_became_ready":    {},
	"meeting_status":                {colMeetingStatus},
	"meeting_status_cleared":        {colMeetingStatus},
	"occurred_at":                   {"occurred_at"},
	"occurred_precision":            {"occurred_precision"},
	"outbound_attested":             {colDirection, "counterparty_email"},
	"project_filing_undone":         {"retention_class"},
	"relinked":                      {participantRows},
	"remind_at":                     {colRemindAt},
	"remind_at_cleared":             {colRemindAt},
	"request_settlement_changed":    {},
	"restored":                      {"archived_at"},
	"source_activity_id":            {colSourceActivity},
	"subject":                       {"subject"},
	"task_type":                     {"task_type"},
	"thread_key":                    {"thread_key"},
}

// activityUpdateCanChangeEdges is false only when the event names its changed
// fields and every one of them is a write the folds do not read. An event with
// no field list, a malformed payload or a key this file has not placed folds
// as before.
func activityUpdateCanChangeEdges(env events.Envelope) bool {
	var payload struct {
		ChangedFields map[string]json.RawMessage `json:"changed_fields"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil || len(payload.ChangedFields) == 0 {
		return true
	}
	for key := range payload.ChangedFields {
		columns, known := updatedFieldWrites[key]
		if !known {
			return true
		}
		for _, column := range columns {
			if foldReadsActivityColumns[column] {
				return true
			}
		}
	}
	return false
}
