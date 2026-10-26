// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

var turnsNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func turnRow(kind crmcontracts.ActivityKind, direction crmcontracts.ActivityDirection, hoursAgo int, body string) crmcontracts.Activity {
	subject := "Pricing"
	row := crmcontracts.Activity{
		Id:         openapi_types.UUID(ids.NewV7()),
		Kind:       kind,
		Subject:    &subject,
		Body:       &body,
		OccurredAt: turnsNow.Add(-time.Duration(hoursAgo) * time.Hour),
	}
	if direction != "" {
		row.Direction = &direction
	}
	return row
}

func TestConversationGivesOurOwnMailItsTextAndMarksANoteAsContext(t *testing.T) {
	t.Parallel()
	out := crmcontracts.ActivityDirectionOutbound
	rows := []crmcontracts.Activity{
		turnRow(crmcontracts.ActivityKindEmail, out, 2, "Second ask: confirm the seats."),
		turnRow(crmcontracts.ActivityKindEmail, out, 20, "First ask: may we send a quote?"),
		turnRow(crmcontracts.ActivityKindNote, "", 30, "Office move on Friday."),
	}
	turns := draftcore.Conversation(rows, nil, turnsNow, time.UTC, 6)
	if len(turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(turns))
	}
	if turns[0].Text != "Second ask: confirm the seats." || turns[1].Text != "First ask: may we send a quote?" {
		t.Errorf("our mails carry %q and %q", turns[0].Text, turns[1].Text)
	}
	if !turns[2].Logged || turns[0].Logged || turns[2].Text != "Office move on Friday." {
		t.Errorf("the note must be marked as context with its text: %+v", turns[2])
	}
}

func TestConversationSkipsWithheldRowsTheAnchorAndNoContactNotes(t *testing.T) {
	t.Parallel()
	out := crmcontracts.ActivityDirectionOutbound
	withheld := crmcontracts.ActivityContentStateWithheld
	hidden := turnRow(crmcontracts.ActivityKindEmail, out, 1, "private text")
	hidden.ContentState = &withheld
	anchor := turnRow(crmcontracts.ActivityKindEmail, out, 5, "the selected message")
	future := turnRow(crmcontracts.ActivityKindMeeting, "", -48, "meeting not yet held")
	rows := []crmcontracts.Activity{hidden, future, turnRow(crmcontracts.ActivityKindEmail, out, 3, "newer"), anchor}
	turns := draftcore.Conversation(rows, &anchor, turnsNow, time.UTC, 6)
	if len(turns) != 1 || turns[0].Text != "newer" || !turns[0].AfterAnchor {
		t.Fatalf("turns = %+v, want only the newer mail, marked after the anchor", turns)
	}
}

func TestConversationBoundsEveryTurnAndTheirNumber(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("word ", 1000)
	var rows []crmcontracts.Activity
	for hours := range 10 {
		direction := crmcontracts.ActivityDirectionInbound
		if hours%2 == 0 {
			direction = crmcontracts.ActivityDirectionOutbound
		}
		rows = append(rows, turnRow(crmcontracts.ActivityKindEmail, direction, hours+1, long))
	}
	turns := draftcore.Conversation(rows, nil, turnsNow, time.UTC, 6)
	if len(turns) != 6 {
		t.Fatalf("got %d turns, want the limit of 6", len(turns))
	}
	for _, turn := range turns {
		if got := len([]rune(turn.Text)); got == 0 || got > draftcore.TurnTextRunes {
			t.Errorf("%s turn has %d runes, want 1 to %d", turn.Direction, got, draftcore.TurnTextRunes)
		}
	}
}

// In Auckland a day note dated today has happened, though its stored noon is
// ahead. A day meeting dated today has not until its date ends.
func TestADayTouchDatedTodayIsReadByItsDate(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 9, 2, 8, 0, 0, 0, auckland)
	today := occurredday.AnchorIn(morning, auckland)
	for kind, want := range map[crmcontracts.ActivityKind]bool{
		crmcontracts.ActivityKindNote:    true,
		crmcontracts.ActivityKindMeeting: false,
	} {
		row := turnRow(kind, "", 0, "Met at the fair")
		row.OccurredAt, row.OccurredPrecision = today, crmcontracts.OccurredPrecisionOccurredPrecisionDay
		if got := draftcore.IsLoggedTouch(row, morning, auckland); got != want {
			t.Errorf("a day %s dated today is a logged touch = %v, want %v", kind, got, want)
		}
	}
}
