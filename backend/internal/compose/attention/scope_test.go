// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Whose day the queue answers, and what happens when a reader asks for one
// that is not theirs to see.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readerAt(tier principal.RowScope) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-000000000001"),
		Permissions: principal.Permissions{RowScope: tier},
	})
}

// A rep sees their own work. The default matters more than it looks: an admin
// account can read every deal in the installation, and a queue that showed all
// of them would hand a rep several hundred colleagues' rows and call it a day.
func TestEveryReaderDefaultsToTheirOwnWork(t *testing.T) {
	for _, tier := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll} {
		scope, err := resolveScope(readerAt(tier), "")
		if err != nil {
			t.Fatalf("row scope %q was refused its own default: %v", tier, err)
		}
		if scope != scopeMine {
			t.Fatalf("row scope %q defaulted to %q, wanted mine", tier, scope)
		}
	}
}

// A reader is offered exactly the scopes their row scope reaches, so a client
// never draws a control that would 403 when pressed.
func TestTheOfferedScopesMatchTheReadersOwnReach(t *testing.T) {
	cases := map[principal.RowScope][]string{
		principal.RowScopeOwn:  {scopeMine},
		principal.RowScopeTeam: {scopeMine, scopeTeam},
		principal.RowScopeAll:  {scopeMine, scopeTeam, scopeAll},
	}
	for tier, want := range cases {
		got := scopeOptionsFor(readerAt(tier))
		if len(got) != len(want) {
			t.Fatalf("row scope %q was offered %v, wanted %v", tier, got, want)
		}
		for i, option := range want {
			if got[i] != option {
				t.Fatalf("row scope %q was offered %v, wanted %v", tier, got, want)
			}
		}
	}
}

// Asking for a scope the reader does not hold is REFUSED, never narrowed.
// Quietly answering a question about the team with facts about one person
// would leave the reader believing they had seen the team.
func TestAWiderScopeThanTheReaderHoldsIsRefusedNotNarrowed(t *testing.T) {
	if _, err := resolveScope(readerAt(principal.RowScopeOwn), scopeTeam); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an own-scope reader asking for the team got %v, wanted a refusal", err)
	}
	if _, err := resolveScope(readerAt(principal.RowScopeTeam), scopeAll); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a team-scope reader asking for everything got %v, wanted a refusal", err)
	}
}

// A team-scoped reader may ask for the team, and an all-scoped reader for
// everything. The refusal above must not be a refusal of everyone.
func TestAReaderMayAskForAScopeTheyDoHold(t *testing.T) {
	if _, err := resolveScope(readerAt(principal.RowScopeTeam), scopeTeam); err != nil {
		t.Fatalf("a team-scope reader was refused the team: %v", err)
	}
	if _, err := resolveScope(readerAt(principal.RowScopeAll), scopeAll); err != nil {
		t.Fatalf("an all-scope reader was refused everything: %v", err)
	}
}

// Under `mine`, a colleague's deal leaves the queue and the summary follows it
// out — a figure counting rows the reader cannot see is the same lie as showing
// them.
func TestAColleaguesDealLeavesTheQueueAndTheSummaryUnderMine(t *testing.T) {
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000ff")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	rows := []ranked{
		{item: ownedDeal("mine", reader)},
		{item: ownedDeal("theirs", colleague)},
	}

	kept := keepReadersOwn(ctx, rows)

	if len(kept) != 1 || kept[0].item.Id != "mine" {
		t.Fatalf("kept %d rows, wanted only the reader's own", len(kept))
	}
}

// A call with no human behind it has no own work to answer for. Handing it
// every row would widen a scope named `mine`, so it gets nothing.
func TestAScopeWithNoReaderBehindItAnswersNothing(t *testing.T) {
	rows := []ranked{{item: ownedDeal("someones", ids.MustParse("01a05500-0000-7000-8000-0000000000ff"))}}

	if kept := keepReadersOwn(context.Background(), rows); len(kept) != 0 {
		t.Fatalf("a caller with no human behind it was handed %d rows under `mine`", len(kept))
	}
}

// Narrowing runs before the page is cut, so a reader asking for three of their
// own rows gets three where they exist — not a short page with their own work
// sitting just past the cut.
func TestAPageOfTheReadersOwnIsNotShortenedByColleaguesRows(t *testing.T) {
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000ff")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	at := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		at = append(at, dealItem("theirs-"+string(rune('a'+i)), colleague))
	}
	for i := 0; i < 3; i++ {
		at = append(at, dealItem("mine-"+string(rune('a'+i)), reader))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: &at}

	out := (&Service{}).worklistFrom(ctx, day, scopeMine, "", 3)

	if len(out.Queue) != 3 {
		t.Fatalf("a reader with three of their own rows got a page of %d", len(out.Queue))
	}
	for _, row := range out.Queue {
		if row.Id[:4] != "mine" {
			t.Fatalf("a colleague's row %q reached a page scoped to the reader", row.Id)
		}
	}
}

func dealItem(id string, owner ids.UUID) crmcontracts.AttentionItem {
	uuid := openapi_types.UUID(owner)
	return crmcontracts.AttentionItem{
		Id:      id,
		Source:  "deal_at_risk",
		Deal:    &crmcontracts.AttentionDealFacts{OwnerId: &uuid},
		Actions: []crmcontracts.AttentionItemActions{},
	}
}

// A row carrying no owner is the reader's by the lane that produced it — their
// own task, their own mailbox, a decision they may make. Dropping those would
// hide the reader's own work from them.
func TestARowWithNoOwnerStaysUnderMine(t *testing.T) {
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	})
	rows := []ranked{{item: crmcontracts.WorklistItem{
		Id: "my-task", Source: "task", Actions: []crmcontracts.WorklistItemActions{},
	}}}

	if len(keepReadersOwn(ctx, rows)) != 1 {
		t.Fatal("a row with no owner was dropped, hiding the reader's own work")
	}
}

func ownedDeal(id string, owner ids.UUID) crmcontracts.WorklistItem {
	uuid := openapi_types.UUID(owner)
	return crmcontracts.WorklistItem{
		Id:      id,
		Source:  "deal_at_risk",
		Deal:    &crmcontracts.WorklistDealFacts{OwnerId: &uuid},
		Actions: []crmcontracts.WorklistItemActions{},
	}
}

// The narrowing has to reach the QUERY. Filtering the answer instead lets a
// colleague's twelve tasks fill a page bounded at twelve and leave the reader's
// own overdue task unreachable behind them — the row they most needed.
func TestTheReadersOwnScopeReachesTheTaskQuery(t *testing.T) {
	tasks := &stubTasks{}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
	)

	if _, err := svc.forReader().Assemble(context.Background()); err != nil {
		t.Fatalf("assembling the day: %v", err)
	}

	if !tasks.mineOnly {
		t.Fatal("the task lane was asked for every visible task on a read scoped to the reader")
	}
}

// And the lane feed keeps its own behaviour: /attention is a different surface
// with its own promise, and this change must not narrow it.
func TestTheLaneFeedStillReadsEveryVisibleTask(t *testing.T) {
	tasks := &stubTasks{}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
	)

	if _, err := svc.Assemble(context.Background()); err != nil {
		t.Fatalf("assembling the day: %v", err)
	}

	if tasks.mineOnly {
		t.Fatal("the lane feed narrowed to the reader, changing a surface this did not set out to change")
	}
}
