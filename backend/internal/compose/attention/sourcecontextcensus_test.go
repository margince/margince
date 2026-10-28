// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// These are the context surfaces exercised by the frontend's generated-source census.
// gatekit:fixture expected surfaces for the assembled attention producers
var contextSurfaceBySource = map[crmcontracts.WorklistItemSource]string{
	"task": "activity", "meeting": "activity", "meeting_outcome": "activity",
	"conversation_claim": "source", "awaiting_reply": "source", "meeting_follow_up": "source",
	"customer_waiting": "record", "lead_response": "record", "brief_item": "record",
	"deal_at_risk": "record", "relationship_decay": "record", "bounce": "record",
	"undelivered": "record", "notice_case": "record", "introduction_request": "record",
	"approval": "inline", "dedupe_candidate": "inline", "deal_suggestion": "inline",
	"tag_suggestion": "inline", "failed_approval": "inline", "domain_question": "inline",
	"notice": "inline", "weekly_commitment": "inline",
	"dsr": "queue", "capture_health": "queue", "ai_work_health": "queue", "automation_run": "queue",
	"batch": "group",
}

func TestProducedWorklistSourcesKeepTheirContext(t *testing.T) {
	day := aDayWithEveryLaneCarryingARow(t)
	rows := classifyDay(day, day.AsOf, dayMoney{})
	rows = append(rows, relationshipCandidates(day)...)
	contact, activity := ids.NewV7(), ids.NewV7()
	rows = append(rows, classifyWaiting(WaitingCustomer{ActivityID: activity, ContactID: contact, Since: day.AsOf}, day.AsOf))
	awaited := AwaitedReply{ActivityID: activity, ContactID: contact, SentAt: day.AsOf}
	rows = append(rows, classifyAwaiting(awaited, day.AsOf, time.UTC))
	meeting, ok := classifyMeetingFollowUp(awaited, day.AsOf, time.UTC)
	if !ok {
		t.Fatal("meeting fixture produced no context")
	}
	rows = append(rows, meeting)
	rows = append(rows, planRowsOf([]PlanWork{{ID: ids.NewV7(), Label: "Confirm scope", DueAt: day.AsOf}}, day.AsOf)...)
	rows = append(rows, batchRow("uncertain_contact", "", rows[:1], false))
	reached := map[crmcontracts.WorklistItemSource]bool{}
	for _, produced := range rows {
		row := produced.item
		reached[row.Source] = true
		surface, known := contextSurfaceBySource[row.Source]
		if !known {
			t.Errorf("source %s has no context surface", row.Source)
			continue
		}
		switch surface {
		case "activity":
			if id, err := ids.Parse(row.Id); err != nil || id.IsZero() {
				t.Errorf("%s lost its activity id: %s", row.Source, row.Id)
			}
		case "source":
			if row.SourceRecord == nil || row.SourceRecord.Type != "activity" || ids.UUID(row.SourceRecord.Id).IsZero() {
				t.Errorf("%s lost its source: %+v", row.Source, row.SourceRecord)
			}
		case "record":
			if row.Subject == nil || ids.UUID(row.Subject.Id).IsZero() {
				t.Errorf("%s lost its record: %+v", row.Source, row.Subject)
			}
		case "group":
			if row.Batch == nil {
				t.Error("batch lost its group context")
			}
		}
	}
	declared := declaredWorklistSources(t)
	if len(contextSurfaceBySource) != len(declared) {
		t.Fatalf("context census has %d sources; contract has %d", len(contextSurfaceBySource), len(declared))
	}
	for _, source := range declared {
		if _, known := contextSurfaceBySource[source]; !known {
			t.Errorf("contract source %s is missing", source)
		}
		// Bare leads remain in the public contract for saved snapshots; no producer emits them.
		if source != sourceLeadResponse && !reached[source] {
			t.Errorf("no production renderer exercised for %s", source)
		}
	}
}
