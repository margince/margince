// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"slices"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// owedAfter is a meeting three days back the reader still owes a follow-up.
func owedAfter(recipients ...ids.UUID) activities.AwaitingReply {
	met := activities.AwaitingReply{
		ActivityID: ids.NewV7(), Subject: "Pricing review", SentAt: testNow.AddDate(0, 0, -3), Recipients: recipients,
	}
	if len(recipients) > 0 {
		met.ContactID = recipients[0]
	}
	return met
}

func TestAFollowUpOwedAfterAMeetingOutranksAnOpenTask(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	met := owedAfter(first, second)
	f := facts{
		deal: openDeal(), now: testNow, loc: time.UTC, followUps: []activities.AwaitingReply{met},
		openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Agree the next step on Nordwind"}},
	}
	mv := decideMove(f)
	if mv.Action != ActionDraftEmail {
		t.Fatalf("action = %q, want the follow-up drafted ahead of the open task", mv.Action)
	}
	if named, ok := operandActivity(mv); !ok || named != met.ActivityID {
		t.Errorf("the move names %s, want the meeting %s", named, met.ActivityID)
	}
	if got := NamedContacts(mv); !slices.Equal(got, []ids.UUID{first, second}) {
		t.Errorf("the move addresses %v, want everyone the meeting was with", got)
	}
	if len(mv.Evidence) != 1 || mv.Evidence[0].ActivityId == nil ||
		*mv.Evidence[0].ActivityId != openapi_types.UUID(met.ActivityID) {
		t.Errorf("evidence = %+v, want the meeting", mv.Evidence)
	}
}

func TestABookedMeetingStillOutranksAFollowUpOwed(t *testing.T) {
	f := facts{
		deal: openDeal(), now: testNow, loc: time.UTC, nextMeeting: booked(testNow.AddDate(0, 0, 2)),
		followUps: []activities.AwaitingReply{owedAfter(ids.NewV7())},
	}
	if mv := decideMove(f); mv.Action != ActionOpenMeetingBrief {
		t.Fatalf("action = %q, want the brief for the meeting booked in two days", mv.Action)
	}
}

func TestAFollowUpWithNobodyOnFileLeavesTheMoveToTheNextRung(t *testing.T) {
	task := ids.NewV7()
	f := facts{
		deal: openDeal(), now: testNow, loc: time.UTC, followUps: []activities.AwaitingReply{owedAfter()},
		openTasks: []activities.OpenTask{{ID: task, Subject: "Send the deck"}},
	}
	if mv := decideMove(f); mv.Action != ActionOpenTask {
		t.Fatalf("action = %q, want the open task: a meeting with nobody to write to has no draft", mv.Action)
	}
}

// The recipients are part of the cache key, so a new attendee on file
// rewrites the card instead of serving the old To line.
func TestTheFollowUpCacheKeyNamesItsRecipients(t *testing.T) {
	recipient := ids.NewV7()
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, followUps: []activities.AwaitingReply{owedAfter(recipient)}}
	if key := moveKey(decideMove(f)); !strings.Contains(key, recipient.String()) {
		t.Fatalf("move key %q does not name the recipient %s", key, recipient)
	}
}

// A meeting renamed or moved keeps its id, so the subject and the time it
// happened are part of the cache key too.
func TestTheFollowUpCacheKeyMovesWithTheMeetingsSubjectAndTime(t *testing.T) {
	met := owedAfter(ids.NewV7())
	keyOf := func(met activities.AwaitingReply) string {
		return moveKey(decideMove(facts{deal: openDeal(), now: testNow, loc: time.UTC, followUps: []activities.AwaitingReply{met}}))
	}
	renamed, moved := met, met
	renamed.Subject = "Contract review"
	moved.SentAt = met.SentAt.Add(-time.Hour)
	if keyOf(renamed) == keyOf(met) || keyOf(moved) == keyOf(met) {
		t.Fatalf("the cache key did not move with the meeting: %q", keyOf(met))
	}
}
