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
	mine := ownedDeal("mine", reader)
	theirs := ownedDeal("theirs", colleague)
	out := crmcontracts.Worklist{
		Queue:   []crmcontracts.WorklistItem{mine, theirs},
		Summary: crmcontracts.WorklistSummary{Total: 2},
	}

	narrowed := narrowToReader(ctx, out)

	if len(narrowed.Queue) != 1 || narrowed.Queue[0].Id != "mine" {
		t.Fatalf("kept %d rows, wanted only the reader's own", len(narrowed.Queue))
	}
	if narrowed.Summary.Total != 1 {
		t.Fatalf("the summary still counts %d, over a queue of 1", narrowed.Summary.Total)
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
	out := crmcontracts.Worklist{
		Queue: []crmcontracts.WorklistItem{{Id: "my-task", Source: "task", Actions: []crmcontracts.WorklistItemActions{}}},
	}

	if len(narrowToReader(ctx, out).Queue) != 1 {
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
