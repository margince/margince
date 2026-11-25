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
		got := MoveRefusal(ctx, action)
		if got == nil || *got != roleReadsOnly["en"] {
			t.Errorf("%s: refusal %v, want the role sentence", action, got)
		}
	}
	for _, action := range []crmcontracts.WorklistMoveAction{
		crmcontracts.WorklistMoveActionOpenTask,
		crmcontracts.WorklistMoveActionOpenMeetingBrief,
		crmcontracts.WorklistMoveActionReconnect,
	} {
		if got := MoveRefusal(ctx, action); got != nil {
			t.Errorf("%s only reads, and was refused: %q", action, *got)
		}
	}
}

func TestASeatThatMayNotWriteIsRefusedEvenWithTheGrant(t *testing.T) {
	got := MoveRefusal(seated(principal.SeatRead, writer), crmcontracts.WorklistMoveActionCreateTask)
	if got == nil || *got != roleReadsOnly["en"] {
		t.Fatalf("a read seat holding activity.create got %v for create_task, want the role sentence", got)
	}
}

func TestAWriterIsRefusedNothing(t *testing.T) {
	ctx := reader(writer)
	for action := range worklistVerbWrites {
		if got := MoveRefusal(ctx, action); got != nil {
			t.Errorf("%s: a writer was refused: %q", action, *got)
		}
	}
}

func TestAnUnclassifiedMoveIsRefusedLikeAWrite(t *testing.T) {
	if MoveRefusal(reader(readerOnly), "send_contract") == nil {
		t.Fatal("a verb the map does not know was offered live to a reader")
	}
	if got := MoveRefusal(reader(writer), "send_contract"); got != nil {
		t.Fatalf("a writer was refused an unknown verb: %q", *got)
	}
}

func TestTheRefusalSpeaksTheReadersLanguage(t *testing.T) {
	ctx := systemtext.WithLocale(reader(readerOnly), textlang.German)
	got := MoveRefusal(ctx, crmcontracts.WorklistMoveActionCreateTask)
	if want := roleReadsOnly["de"]; got == nil || *got != want {
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
		if got := row.Action.BlockedReason; got == nil || *got != roleReadsOnly["en"] {
			t.Errorf("%s: %v, want the role sentence", row.Action.Kind, got)
		}
	}
	if got := rows[2].Action.BlockedReason; got != nil {
		t.Errorf("open_deal only reads, and was refused: %q", *got)
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
	got := MoveRefusal(asWho(readerOnly), crmcontracts.WorklistMoveActionBringInManager)
	if got == nil || *got != "You do not have permission to change deals" {
		t.Errorf("a reader of deals who may write activities was offered the move live: %v", got)
	}
	if got := MoveRefusal(asWho(principal.ObjectGrant{Read: true, Update: true}), crmcontracts.WorklistMoveActionBringInManager); got != nil {
		t.Errorf("a seat that may update deals was refused: %q", *got)
	}
}

func TestAskingTheManagerForARoleThatOnlyReadsSaysTheRoleSentence(t *testing.T) {
	got := MoveRefusal(reader(readerOnly), crmcontracts.WorklistMoveActionBringInManager)
	if got == nil || *got != roleReadsOnly["en"] {
		t.Errorf("a role that only reads got %v, want the role sentence", got)
	}
}
