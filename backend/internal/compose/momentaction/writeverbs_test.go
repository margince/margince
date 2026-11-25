// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var (
	writer     = principal.ObjectGrant{Read: true, Create: true}
	readerOnly = principal.ObjectGrant{Read: true}
)

func TestAReaderIsRefusedEveryMoveThatWritesAndNoneThatReads(t *testing.T) {
	ctx := reader(readerOnly)
	for _, action := range []crmcontracts.WorklistMoveAction{
		crmcontracts.WorklistMoveActionDraftReply,
		crmcontracts.WorklistMoveActionDraftEmail,
		crmcontracts.WorklistMoveActionCreateTask,
	} {
		got := MoveRefusal(ctx, "contact", action)
		if got == nil || *got != RoleReadsOnly().En {
			t.Errorf("%s: refusal %v, want the role sentence", action, got)
		}
	}
	for _, action := range []crmcontracts.WorklistMoveAction{
		crmcontracts.WorklistMoveActionOpenTask,
		crmcontracts.WorklistMoveActionOpenMeetingBrief,
		crmcontracts.WorklistMoveActionReconnect,
	} {
		if got := MoveRefusal(ctx, "contact", action); got != nil {
			t.Errorf("%s only reads, and was refused: %q", action, *got)
		}
	}
}

func TestASeatThatMayNotWriteIsRefusedEvenWithTheGrant(t *testing.T) {
	got := MoveRefusal(seated(principal.SeatRead, writer), "deal", crmcontracts.WorklistMoveActionCreateTask)
	if got == nil || *got != SeatReadsOnly().En {
		t.Fatalf("a read seat holding activity.create got %v for create_task, want the seat sentence", got)
	}
}

func TestARoleThatUpdatesTheRowButWritesNoActivityHearsTheActionSentence(t *testing.T) {
	for _, record := range []string{"contact", "deal", ""} {
		got := MoveRefusal(customRole(), record, crmcontracts.WorklistMoveActionDraftEmail)
		if got == nil || *got != ActionRefused().En {
			t.Errorf("a row about %q: refusal %v, want the action sentence", record, got)
		}
	}
}

func TestARowAboutNoRecordNeverBlamesTheRole(t *testing.T) {
	got := MoveRefusal(reader(readerOnly), "", crmcontracts.WorklistMoveActionCreateTask)
	if got == nil || *got != ActionRefused().En {
		t.Fatalf("a row about no record got %v, want the action sentence", got)
	}
}

func TestAWriterIsRefusedNothing(t *testing.T) {
	ctx := reader(writer)
	for action := range worklistVerbWrites {
		if got := MoveRefusal(ctx, "contact", action); got != nil {
			t.Errorf("%s: a writer was refused: %q", action, *got)
		}
	}
}

func TestAnUnclassifiedMoveIsRefusedLikeAWrite(t *testing.T) {
	if MoveRefusal(reader(readerOnly), "contact", "send_contract") == nil {
		t.Fatal("a verb the map does not know was offered live to a reader")
	}
	if got := MoveRefusal(reader(writer), "contact", "send_contract"); got != nil {
		t.Fatalf("a writer was refused an unknown verb: %q", *got)
	}
}

func TestTheRefusalSpeaksTheReadersLanguage(t *testing.T) {
	ctx := systemtext.WithLocale(reader(readerOnly), textlang.German)
	got := MoveRefusal(ctx, "contact", crmcontracts.WorklistMoveActionCreateTask)
	if want := RoleReadsOnly().De; got == nil || *got != want {
		t.Fatalf("refusal %v, want %q", got, want)
	}
}

func TestACompanyBriefRowBlocksOnlyItsWritingAction(t *testing.T) {
	rows := []crmcontracts.Company360Suggestion{
		suggestion(crmcontracts.Company360SuggestionActionKindAddTask),
		suggestion(crmcontracts.Company360SuggestionActionKindDraftReply),
		suggestion(crmcontracts.Company360SuggestionActionKindOpenDeal),
		{},
	}
	MarkRefusedSuggestions(reader(readerOnly), rows)
	for _, row := range rows[:2] {
		if got := row.Action.BlockedReason; got == nil || *got != RoleReadsOnly().En {
			t.Errorf("%s: %v, want the role sentence", row.Action.Kind, got)
		}
	}
	if got := rows[2].Action.BlockedReason; got != nil {
		t.Errorf("open_deal only reads, and was refused: %q", *got)
	}
	MarkRefusedSuggestions(customRole(), rows)
	for _, row := range rows[:2] {
		if got := row.Action.BlockedReason; got == nil || *got != ActionRefused().En {
			t.Errorf("%s for a role that updates the company: %v, want the action sentence", row.Action.Kind, got)
		}
	}
	MarkRefusedSuggestions(reader(writer), rows)
	for i, row := range rows[:3] {
		if row.Action.BlockedReason != nil {
			t.Errorf("row %d: a writer was refused: %q", i, *row.Action.BlockedReason)
		}
	}
}

// suggestion builds a row whose generated action type this test never spells.
func suggestion(kind crmcontracts.Company360SuggestionActionKind) crmcontracts.Company360Suggestion {
	var row crmcontracts.Company360Suggestion
	row.Action = newOf(row.Action)
	row.Action.Kind = kind
	return row
}

func newOf[T any](*T) *T { return new(T) }

func TestAskingTheManagerAsksTheDealGrantNotTheActivityGrant(t *testing.T) {
	asWho := func(deal principal.ObjectGrant) context.Context {
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(), SeatType: principal.SeatFull,
			Permissions: principal.Permissions{
				RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
				Objects: map[string]principal.ObjectGrant{"deal": deal, "activity": writer},
			},
		})
	}
	got := MoveRefusal(asWho(readerOnly), "deal", crmcontracts.WorklistMoveActionBringInManager)
	if got == nil || *got != RoleReadsOnly().En {
		t.Errorf("a reader of deals who may write activities got %v, want the role sentence", got)
	}
	if got := MoveRefusal(asWho(principal.ObjectGrant{Read: true, Update: true}), "deal", crmcontracts.WorklistMoveActionBringInManager); got != nil {
		t.Errorf("a seat that may update deals was refused: %q", *got)
	}
}

func TestAskingTheManagerOnAReadSeatSaysTheSeatSentence(t *testing.T) {
	got := MoveRefusal(seated(principal.SeatRead, writer), "deal", crmcontracts.WorklistMoveActionBringInManager)
	if got == nil || *got != SeatReadsOnly().En {
		t.Errorf("a read seat got %v, want the seat sentence", got)
	}
}
