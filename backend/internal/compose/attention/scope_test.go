// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Whose day the queue answers, and what happens when a reader asks for one
// that is not theirs to see.

import (
	"context"
	"errors"
	"testing"
	"time"

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
	// Unassigned is offered at EVERY tier: nothing in it belongs to a
	// colleague, and it is where ownerless work lives now that "mine" no longer
	// folds it into each reader's own queue.
	cases := map[principal.RowScope][]string{
		principal.RowScopeOwn:  {scopeMine, scopeUnassigned},
		principal.RowScopeTeam: {scopeMine, scopeUnassigned, scopeTeam},
		principal.RowScopeAll:  {scopeMine, scopeUnassigned, scopeTeam, scopeAll},
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

	out := (&Service{}).worklistFrom(ctx, day, scopeMine, "", 3, nil, leadRead{})

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

	if !tasks.mineOnly() {
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

	if tasks.mineOnly() {
		t.Fatal("the lane feed narrowed to the reader, changing a surface this did not set out to change")
	}
}

// Opening somebody else's queue is a team-or-wider question.
//
// Both halves, because either alone proves nothing: a resolver that refused
// everybody would pass the refusal test while making the feature dead, and one
// that admitted everybody would pass the admission test while handing a rep
// their colleague's day.
func TestOpeningAnothersQueueNeedsATierThatReachesThem(t *testing.T) {
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")
	svc := &Service{teammates: teammatesSaying(true)}

	for _, tier := range []principal.RowScope{principal.RowScopeTeam, principal.RowScopeAll} {
		got, err := svc.resolveOwner(readerAt(tier), colleague)
		if err != nil {
			t.Fatalf("row scope %q was refused a colleague's queue: %v", tier, err)
		}
		if got != colleague {
			t.Fatalf("row scope %q was given %v, wanted the named colleague", tier, got)
		}
	}

	if _, err := svc.resolveOwner(readerAt(principal.RowScopeOwn), colleague); err == nil {
		t.Fatal("a reader whose scope reaches only themselves opened a colleague's queue")
	}
}

// A team-scoped reader reaches their own team, and stops there.
//
// The tier alone is not the test for this reader class. Row scope narrows the
// deal-bearing rows, but a task carrying no record link is discoverable by
// anyone (auth.ActivityDiscoverClause coalesces the empty link set to TRUE), so
// naming an out-of-team colleague would answer with exactly those rows under
// that colleague's name.
func TestATeamScopedReaderReachesOnlyTheirOwnTeam(t *testing.T) {
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")

	shared := &Service{teammates: teammatesSaying(true)}
	got, err := shared.resolveOwner(readerAt(principal.RowScopeTeam), colleague)
	if err != nil {
		t.Fatalf("a team-scoped reader was refused a teammate's queue: %v", err)
	}
	if got != colleague {
		t.Fatalf("a teammate's queue answered %v, wanted the named colleague", got)
	}

	stranger := &Service{teammates: teammatesSaying(false)}
	if _, err := stranger.resolveOwner(readerAt(principal.RowScopeTeam), colleague); err == nil {
		t.Fatal("a team-scoped reader opened the queue of somebody on no team of theirs")
	}
}

// An unanswerable may-I is a no.
//
// Every other optional lane renders absent when unbound, which is why this one
// needs saying: a membership lane that degraded the same way would widen the
// scope it exists to bound, and would look like a missing feature while doing
// it.
func TestAnUnboundMembershipLaneRefusesRatherThanAdmits(t *testing.T) {
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")

	unbound := &Service{}
	if _, err := unbound.resolveOwner(readerAt(principal.RowScopeTeam), colleague); err == nil {
		t.Fatal("a team-scoped reader opened a colleague's queue with no membership lane to ask")
	}

	// The unbounded reader is unaffected: they reach every row, so the lane was
	// never part of their answer.
	if _, err := unbound.resolveOwner(readerAt(principal.RowScopeAll), colleague); err != nil {
		t.Fatalf("an unbounded reader was refused for want of a lane they do not need: %v", err)
	}
}

// A membership read that fails is not a membership read that said no.
func TestAFailedMembershipReadRefusesAndReportsTheFailure(t *testing.T) {
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")
	svc := &Service{teammates: teammatesFailing{}}

	if _, err := svc.resolveOwner(readerAt(principal.RowScopeTeam), colleague); err == nil {
		t.Fatal("a failed membership read admitted the reader")
	}
}

// Naming yourself is the question the default already answers, so every tier
// may ask it. A rep following a link that spells out their own id must not be
// refused their own day.
func TestNamingYourselfNeedsNoWiderTier(t *testing.T) {
	me := ids.MustParse("01a05500-0000-7000-8000-000000000001")

	got, err := (&Service{}).resolveOwner(readerAt(principal.RowScopeOwn), me)
	if err != nil {
		t.Fatalf("a reader was refused their OWN queue: %v", err)
	}
	if got != me {
		t.Fatalf("asking for their own queue answered %v", got)
	}
}

// No ask is no narrowing. The parameter is optional, and its absence must not
// read as "nobody's queue" and empty the page.
func TestNoOwnerAskNarrowsNothing(t *testing.T) {
	got, err := (&Service{}).resolveOwner(readerAt(principal.RowScopeAll), ids.UUID{})
	if err != nil {
		t.Fatalf("omitting the owner was refused: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("omitting the owner narrowed to %v", got)
	}
}

// A named rep's queue carries THEIR work, not the reader's.
//
// The per-user lanes — notices, meetings, a mailbox, a promise — stay bound to
// the ACTING reader whatever owner is asked for, because that is where the
// modules that own them bind. So a filter that kept every row it could not
// judge handed a manager their own day with somebody else's name at the top of
// it. Nothing crossed a scope boundary; the page simply was not true.
func TestOpeningAnothersQueueCarriesTheirWorkAndNotTheReadersOwn(t *testing.T) {
	lena := ids.MustParse("01a05500-0000-7000-8000-0000000000bb")
	lenasDeal := item("lenas-deal", "deal_at_risk", withDeal(90_000_00))
	lenasDeal.Deal.OwnerId = uuidPtr(lena)
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		// The reader's own: a notice addressed to them, and a meeting they can
		// see. Neither carries a deal, so neither can be judged by ownership.
		Notices:  lane(item("my-notice", "notice")),
		Meetings: lane(item("my-meeting", "meeting", withDue(rankInstant.Add(time.Hour)))),
		AtRisk:   lane(lenasDeal),
	}
	reader := &Service{taskOwner: lena, taskScope: TasksOwnedBy}

	out := reader.worklistFrom(context.Background(), day, scopeMine, "", 25, nil, leadRead{})

	var ids []string
	for _, row := range out.Queue {
		ids = append(ids, row.Id)
	}
	if len(out.Queue) != 1 || out.Queue[0].Id != "lenas-deal" {
		t.Fatalf("Lena's queue came back as %v, wanted only the deal she owns", ids)
	}
}

func uuidPtr(id ids.UUID) *openapi_types.UUID {
	out := openapi_types.UUID(id)
	return &out
}

// teammatesSaying answers every membership question the same way, which is what
// the resolver's own branches need: whether it ASKS, and what it does with each
// answer.
type teammatesSaying bool

func (t teammatesSaying) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return bool(t), nil
}

// The roster half answers the reader alone, which is what a caller on no team
// gets from the real reader. These tests are about the yes/no half.
func (t teammatesSaying) LiveTeammatesOfCaller(context.Context) ([]TeamMember, error) {
	return []TeamMember{{UserID: ids.UUID{1}, DisplayName: "the reader"}}, nil
}

// teammatesFailing is the membership read that could not answer.
type teammatesFailing struct{}

func (teammatesFailing) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return false, errors.New("reading team membership")
}

func (teammatesFailing) LiveTeammatesOfCaller(context.Context) ([]TeamMember, error) {
	return nil, errors.New("reading team membership")
}
