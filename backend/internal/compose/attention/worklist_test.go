// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the queue says about a day, as opposed to how it orders one.
//
// The projection's promise: every item states what happens if the reader does
// nothing, the figures above the list match the rows below it, and a day never
// reads as clear when something that would have filled it was never read.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func item(id string, source crmcontracts.AttentionItemSource, opts ...func(*crmcontracts.AttentionItem)) crmcontracts.AttentionItem {
	row := crmcontracts.AttentionItem{
		Id:      id,
		Source:  source,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	for _, opt := range opts {
		opt(&row)
	}
	return row
}

func withKind(kind string) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.Kind = &kind }
}

func withDue(at time.Time) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.DueAt = &at }
}

func withDetail(detail string) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.Detail = &detail }
}

func lane(items ...crmcontracts.AttentionItem) *[]crmcontracts.AttentionItem { return &items }

// Every row must answer "what happens if I do nothing?" — the question a queue
// exists to answer and the one the lane feed had no field for.
func TestEveryItemSaysWhatHappensIfTheReaderDoesNothing(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("d1", "approval", withKind("send_email"))},
		Planned:  []crmcontracts.AttentionItem{item("t1", "task", withDue(rankInstant))},
		AtRisk:   lane(item("r1", "deal_at_risk", withDetail("83"))),
		Bounces:  lane(item("b1", "bounce")),
	}

	rows := classifyDay(day, rankInstant)

	if len(rows) == 0 {
		t.Fatal("classified nothing from a day with four items in it")
	}
	for _, row := range rows {
		if row.item.Consequence == "" {
			t.Fatalf("item %q states no consequence, so a reader cannot tell what ignoring it costs", row.item.Id)
		}
	}
}

// One source, two honest answers. A deal past the date the customer agreed to
// SLIPS; one merely idle DRIFTS. Collapsing them tells a reader the wrong thing
// about the deal that actually has a date.
func TestADealPastItsCloseDateSlipsWhileAnIdleOneDrifts(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("late", "deal_at_risk", withKind("close_overdue")), item("idle", "deal_at_risk", withKind("quiet"))),
	}

	rows := classifyDay(day, rankInstant)

	for _, row := range rows {
		want := crmcontracts.WorklistItemConsequence("deal_drifts")
		if row.item.Id == "late" {
			want = "deal_slips_past_close"
		}
		if row.item.Consequence != want {
			t.Fatalf("%q says %q, wanted %q", row.item.Id, row.item.Consequence, want)
		}
	}
}

// A decision about a SEND holds up a customer; a decision about a contact record
// tidies the database. Ranking them alike is what let 188 hygiene questions bury
// the buyer.
func TestASendDecisionOutranksAContactDecision(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("hygiene", "approval", withKind("capture_counterparty")),
			item("send", "approval", withKind("send_email")),
		},
	}

	got := rankAll(classifyDay(day, rankInstant))

	assertOrder(t, got, "send", "hygiene")
	if got[0].Level != levelBlocking || got[1].Level != levelRoutine {
		t.Fatalf("levels were %d and %d, wanted blocking then routine", got[0].Level, got[1].Level)
	}
}

// A meeting two hours out is the most urgent thing on the page, because it
// happens whether or not the reader acts. The same meeting next week is not.
func TestAMeetingWithinTwoHoursLeadsAndALaterOneDoesNot(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Meetings: lane(
			item("next-week", "meeting", withDue(rankInstant.Add(7*24*time.Hour))),
			item("in-an-hour", "meeting", withDue(rankInstant.Add(time.Hour))),
		),
	}

	rows := classifyDay(day, rankInstant)

	for _, row := range rows {
		want := levelAgreed
		if row.item.Id == "in-an-hour" {
			want = levelWaiting
		}
		if row.item.Level != want {
			t.Fatalf("%q landed at level %d, wanted %d", row.item.Id, row.item.Level, want)
		}
	}
}

// The figures above the list have to describe the rows below it. The lane feed
// reported a twelve-item page as a total, which is the defect this replaces.
func TestTheSummaryCountsTheSameItemsTheQueueCarries(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("d1", "approval", withKind("capture_counterparty"))},
		Planned: []crmcontracts.AttentionItem{
			item("t1", "task", withDue(rankInstant.Add(-time.Hour))),
			item("t2", "task", withDue(rankInstant.Add(time.Hour))),
		},
		Commitments: lane(item("c1", "conversation_claim", withDue(rankInstant.Add(-24*time.Hour)))),
	}

	ordered := rankAll(classifyDay(day, rankInstant))
	summary := summarize(ordered)

	if summary.Total != len(ordered) {
		t.Fatalf("summary totals %d over a queue of %d", summary.Total, len(ordered))
	}
	if summary.Urgent != 1 {
		t.Fatalf("counted %d urgent, wanted the one promise", summary.Urgent)
	}
	if summary.LowerPriority != 1 {
		t.Fatalf("counted %d lower-priority, wanted the one hygiene decision", summary.LowerPriority)
	}
}

// A lane the reader may not see is NAMED, so a clear-looking day cannot be a
// day nobody looked at. The DSR lane is the one exception: it is withheld by
// role from every rep permanently, and saying so on every page forever would
// drown the warning this list exists to give.
func TestAWithheldLaneIsNamedButTheRoleWithheldPrivacyLaneIsNot(t *testing.T) {
	omitted := []crmcontracts.AttentionLanesOmitted{"capture_health", "dsr"}
	day := crmcontracts.Attention{AsOf: rankInstant, LanesOmitted: &omitted}

	got := unavailable(day)

	if len(got) != 1 {
		t.Fatalf("named %d sources, wanted only the mailbox one", len(got))
	}
	if got[0].Source != "capture_health" || got[0].Reason != crmcontracts.WorklistSourceUnavailableReasonWithheld {
		t.Fatalf("named %q/%q, wanted capture_health/withheld", got[0].Source, got[0].Reason)
	}
}

// A day with nothing withheld says nothing was withheld — as an empty list, not
// a null a client has to guess about.
func TestADayWithNothingWithheldNamesNothing(t *testing.T) {
	got := unavailable(crmcontracts.Attention{AsOf: rankInstant})

	if got == nil {
		t.Fatal("sent null where the contract promised a list")
	}
	if len(got) != 0 {
		t.Fatalf("named %d sources on a day that withheld none", len(got))
	}
}

// The filter narrows what is carried, and the rows that survive are all of that
// kind. A reader who asked for deals must not be handed decisions.
func TestAFilteredQueueCarriesOnlyThatKindOfWork(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("d1", "approval", withKind("send_email"))},
		AtRisk:   lane(item("r1", "deal_at_risk")),
	}

	kept := keepCategory(classifyDay(day, rankInstant), "deals_at_risk")

	if len(kept) != 1 || kept[0].item.Id != "r1" {
		t.Fatalf("filtering for deals kept %d rows, wanted just the deal", len(kept))
	}
}

// A staged decision's date is its EXPIRY — when the proposal lapses if nobody
// answers — not a deadline the reader owes anything on. Counting those as work
// due today told a rep eleven things were due on a day that held two.
func TestADecisionsExpiryIsNotCountedAsWorkDue(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("expiring", "approval", withKind("capture_counterparty"), withDue(rankInstant.Add(72*time.Hour))),
		},
		Planned: []crmcontracts.AttentionItem{item("real-task", "task", withDue(rankInstant.Add(-time.Hour)))},
	}

	summary := summarize(rankAll(classifyDay(day, rankInstant)))

	if summary.Due != 1 {
		t.Fatalf("counted %d due, wanted only the task — a proposal's expiry is not the reader's deadline", summary.Due)
	}
}

func withDeal(minor int64) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) {
		amount := minor
		i.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &amount}
	}
}

func withOverdue(past bool) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.Overdue = &past }
}

// The deal's own figures have to reach the row, or the client reads a second
// endpoint per line to draw a card this one could have completed.
func TestARiskRowCarriesTheDealsFigures(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk", withDeal(160_100_00))),
	}

	rows := classifyDay(day, rankInstant)

	if rows[0].item.Deal == nil || rows[0].item.Deal.AmountMinor == nil {
		t.Fatal("the deal facts the lane feed resolved never reached the queue row")
	}
}

// Material revenue interrupts the day; a smaller deal drifting is agreed work.
// The bar is the pipeline's own median, so a one-euro deal cannot claim the
// band reserved for revenue worth stopping for.
func TestOnlyADealAboveTheMedianReachesTheMaterialBand(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("tiny", "deal_at_risk", withDeal(1_00)),
			item("small", "deal_at_risk", withDeal(2_00)),
			item("big", "deal_at_risk", withDeal(160_100_00)),
		),
	}

	rows := classifyDay(day, rankInstant)

	for _, row := range rows {
		want := levelAgreed
		if row.item.Id == "big" {
			want = levelMaterialRisk
		}
		if row.item.Level != want {
			t.Fatalf("%q landed at level %d, wanted %d", row.item.Id, row.item.Level, want)
		}
	}
}

// A deal past the date the customer agreed to outranks one merely quiet for
// longer. Without the close date on the row the lane compared idle days alone,
// and the deal with the real deadline lost.
func TestADealPastItsCloseDateOutranksAQuieterOne(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("quiet-longer", "deal_at_risk", withDetail("20")),
			item("past-close", "deal_at_risk", withKind("close_overdue"),
				withDue(rankInstant.Add(-24*time.Hour)), withOverdue(true), withDetail("1")),
		),
	}

	got := rankAll(classifyDay(day, rankInstant))

	assertOrder(t, got, "past-close", "quiet-longer")
}

// The night's ranked work belongs ON the queue. A lane the ranking never sees
// is a lane the reader was told to read separately, which is the arrangement
// this endpoint exists to end.
func TestTheOvernightBriefingReachesTheQueue(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:        rankInstant,
		ThisMorning: []crmcontracts.AttentionItem{item("brief-1", "brief_item")},
	}

	rows := classifyDay(day, rankInstant)

	if len(rows) != 1 || rows[0].item.Source != "brief_item" {
		t.Fatalf("the briefing lane produced %d queue rows, wanted one", len(rows))
	}
}

// The summary and the last row must describe the page the caller RECEIVED.
// Ranking the whole set and slicing afterwards left the final row comparing
// itself against a row nobody got, and a total longer than the list.
func TestThePageIsSummarisedAndExplainedAgainstItself(t *testing.T) {
	tasks := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		tasks = append(tasks, item(string(rune('a'+i)), "task", withDue(rankInstant.Add(time.Duration(i)*time.Hour))))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: tasks}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 3, nil)

	if out.Summary.Total != len(out.Queue) {
		t.Fatalf("summary totals %d over a page of %d", out.Summary.Total, len(out.Queue))
	}
	if out.Queue[len(out.Queue)-1].AboveNext != nil {
		t.Fatal("the last row on the page compares itself with a row the caller never received")
	}
}

// An outbound send blocks a customer whichever door staged it. Treating one
// spelling as urgent and another as hygiene is how the same act ends up in two
// places in the queue.
func TestEveryOutboundSendKindBlocksCustomerWork(t *testing.T) {
	for _, kind := range []string{"send_email", "send_account_email", "send_message", "book_meeting"} {
		day := crmcontracts.Attention{
			AsOf:     rankInstant,
			NeedsYou: []crmcontracts.AttentionItem{item("a", "approval", withKind(kind))},
		}

		rows := classifyDay(day, rankInstant)

		if rows[0].item.Level != levelBlocking {
			t.Fatalf("%q landed at level %d, wanted the blocking band", kind, rows[0].item.Level)
		}
	}
}

// "Due" means a date that has arrived. A task due later today is agreed work
// the reader has not missed, and counting it told them they were behind.
func TestOnlyAnArrivedDateCountsAsDue(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("late", "task", withDue(rankInstant.Add(-time.Hour))),
			item("later-today", "task", withDue(rankInstant.Add(6*time.Hour))),
		},
	}

	summary := summarize(rankAll(classifyDay(day, rankInstant)))

	if summary.Due != 1 {
		t.Fatalf("counted %d due, wanted only the one whose date has passed", summary.Due)
	}
}

// A two-deal pipeline still has a biggest deal, and it is the material one.
// Taking the upper-middle value left the largest deal below its own median.
func TestTheBiggerOfTwoDealsIsMaterial(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("small", "deal_at_risk", withDeal(2_000_00)),
			item("big", "deal_at_risk", withDeal(160_100_00)),
		),
	}

	rows := classifyDay(day, rankInstant)

	for _, row := range rows {
		want := levelAgreed
		if row.item.Id == "big" {
			want = levelMaterialRisk
		}
		if row.item.Level != want {
			t.Fatalf("%q landed at level %d, wanted %d", row.item.Id, row.item.Level, want)
		}
	}
}

// Somebody waiting on a reply is the top of the day, above every decision and
// every drifting deal. It is the one thing here whose cost of inaction falls on
// somebody else.
func TestAWaitingCustomerLeadsTheDay(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("decision", "approval", withKind("send_email"))},
		AtRisk:   lane(item("drifting", "deal_at_risk", withDeal(500_000_00))),
	}
	waiting := []WaitingCustomer{{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-00000000aaaa"),
		Subject:    "Re: pricing",
		Since:      rankInstant.Add(-83 * 24 * time.Hour),
	}}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, waiting)

	if out.Queue[0].Source != "customer_waiting" {
		t.Fatalf("the day led with %q, not the customer who is waiting", out.Queue[0].Source)
	}
	if out.Queue[0].Consequence != "buyer_waits" {
		t.Fatalf("a waiting customer says %q happens if ignored", out.Queue[0].Consequence)
	}
}

// One unanswered message is ONE row. The deal it belongs to must not also
// appear as drifting, or the rep is asked twice for the same reply.
func TestAWaitingDealDoesNotAlsoAppearAsDrifting(t *testing.T) {
	deal := ids.MustParse("01a05500-0000-7000-8000-00000000bbbb")
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item(deal.String(), "deal_at_risk", withDeal(160_100_00))),
	}
	waiting := []WaitingCustomer{{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-00000000cccc"),
		Since:      rankInstant.Add(-3 * 24 * time.Hour),
		DealID:     deal,
	}}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, waiting)

	if len(out.Queue) != 1 {
		t.Fatalf("one unanswered message produced %d rows", len(out.Queue))
	}
	if out.Queue[0].Source != "customer_waiting" {
		t.Fatalf("the surviving row was %q, wanted the waiting one", out.Queue[0].Source)
	}
}

// The longer somebody has waited, the higher they sit — among people who are
// all waiting, the forgotten one is the one at risk.
func TestTheLongestWaitLeadsAmongWaitingCustomers(t *testing.T) {
	waiting := []WaitingCustomer{
		{ActivityID: ids.MustParse("01a05500-0000-7000-8000-00000000000a"), Since: rankInstant.Add(-2 * 24 * time.Hour)},
		{ActivityID: ids.MustParse("01a05500-0000-7000-8000-00000000000b"), Since: rankInstant.Add(-83 * 24 * time.Hour)},
	}

	out := (&Service{}).worklistFrom(context.Background(), crmcontracts.Attention{AsOf: rankInstant}, scopeAll, "", 25, waiting)

	if out.Queue[0].Id != "01a05500-0000-7000-8000-00000000000b" {
		t.Fatal("the two-day wait outranked the eighty-three-day one")
	}
}

// A message the reader may not read produces NO row.
//
// The earlier cut kept the row and removed only its subject, which still
// published the wait, its timing and the record it was filed under — and let a
// reader watch a row vanish to learn that a reply they may not see had arrived.
// The content gate now decides whether the row exists, so this is a property of
// the query and the store test holds it; here we hold the shape that depends on
// it: every waiting row that arrives may state its subject.
func TestEveryWaitingRowMayStateItsSubject(t *testing.T) {
	waiting := []WaitingCustomer{{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-00000000eeee"),
		Subject:    "Re: pricing",
		Since:      rankInstant.Add(-24 * time.Hour),
	}}

	out := (&Service{}).worklistFrom(context.Background(), crmcontracts.Attention{AsOf: rankInstant}, scopeAll, "", 25, waiting)

	if out.Queue[0].Title == nil || *out.Queue[0].Title != "Re: pricing" {
		t.Fatal("a waiting row the content gate admitted lost its subject line")
	}
}

// Under `mine`, a thread about a colleague's deal is that colleague's to
// answer. A message filed under nothing stays: an unowned customer writing in
// is everybody's, and dropping it would leave nobody looking at it.
func TestAColleaguesWaitingCustomerLeavesTheReadersOwnQueue(t *testing.T) {
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000ff")
	theirDeal := ids.MustParse("01a05500-0000-7000-8000-00000000dea1")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	at := []crmcontracts.AttentionItem{dealItemOwned(theirDeal, colleague)}
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: &at}
	waiting := []WaitingCustomer{
		{ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000a1"), Since: rankInstant.Add(-time.Hour), DealID: theirDeal},
		{ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000a2"), Since: rankInstant.Add(-time.Hour)},
	}

	out := (&Service{}).worklistFrom(ctx, day, scopeMine, "", 25, waiting)

	for _, row := range out.Queue {
		if row.Id == "01a05500-0000-7000-8000-0000000000a1" {
			t.Fatal("a thread about a colleague's deal reached a queue scoped to the reader")
		}
	}
	if len(out.Queue) == 0 {
		t.Fatal("the unfiled message was dropped too, leaving nobody looking at it")
	}
}

func dealItemOwned(deal, owner ids.UUID) crmcontracts.AttentionItem {
	ownerID := openapi_types.UUID(owner)
	dealID := openapi_types.UUID(deal)
	return crmcontracts.AttentionItem{
		Id:      deal.String(),
		Source:  "deal_at_risk",
		Subject: &crmcontracts.AttentionSubject{Type: "deal", Id: dealID},
		Deal:    &crmcontracts.AttentionDealFacts{OwnerId: &ownerID},
		Actions: []crmcontracts.AttentionItemActions{},
	}
}

// A pile of alike questions is one row. On the real workspace this is 152
// contact decisions and 545 held drafts — no ordering saves a reader who must
// scroll past them to reach the next thing.
func TestAPileOfAlikeDecisionsBecomesOneRow(t *testing.T) {
	staged := []crmcontracts.AttentionItem{}
	for i := 0; i < 40; i++ {
		staged = append(staged, item(
			"c"+string(rune('a'+i%26))+string(rune('0'+i/26)),
			"approval", withKind("capture_counterparty")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: staged}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 1 {
		t.Fatalf("forty alike decisions drew %d rows", len(got))
	}
	if got[0].Batch == nil || got[0].Batch.Count != 40 {
		t.Fatal("the row does not say how many decisions it stands for")
	}
}

// Contact questions split by what they are ABOUT, and the split is derived
// from the STAGED PAYLOAD rather than fabricated here — a test that writes the
// marker itself proves nothing about the code that writes it in production.
func TestContactDecisionsSplitByWhatTheyAreAbout(t *testing.T) {
	staged := []crmcontracts.Approval{}
	for i := 0; i < 3; i++ {
		staged = append(staged,
			captureApproval("noreply@vendor.example"),
			captureApproval("anna.weber@customer.example"))
	}
	items := make([]crmcontracts.AttentionItem, 0, len(staged))
	for _, approval := range staged {
		items = append(items, approvalItem(approval, func(address string) bool {
			return strings.HasPrefix(address, "noreply@")
		}))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: items}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	keys := map[crmcontracts.WorklistBatchKey]int{}
	for _, row := range got {
		if row.Batch != nil {
			keys[row.Batch.Key] = row.Batch.Count
		}
	}
	if keys["likely_automated"] != 3 {
		t.Fatalf("the machine group holds %d, wanted 3", keys["likely_automated"])
	}
	if keys["uncertain_contact"] != 3 {
		t.Fatalf("the remainder holds %d, wanted 3", keys["uncertain_contact"])
	}
}

// A company match needs a lookup this assembler does not make. The only
// company-ish field on the payload is the sender's own display name, which
// capture labels untrusted and never-for-matching — so `Alice <alice@gmail.com>`
// must not read as a company we know.
func TestASendersOwnDisplayNameIsNotACompanyMatch(t *testing.T) {
	approval := captureApproval("alice@gmail.com")
	change := map[string]any{"email": "alice@gmail.com", "display_name": "Alice Example"}
	approval.ProposedChange = &change

	item := approvalItem(approval, func(string) bool { return false })

	if item.Detail != nil && strings.Contains(*item.Detail, stagedKnownCompany) {
		t.Fatal("a sender's own display name was taken for a company we know")
	}
}

func captureApproval(email string) crmcontracts.Approval {
	change := map[string]any{"email": email}
	summary := "Is " + email + " a contact worth keeping?"
	return crmcontracts.Approval{
		Id:             openapi_types.UUID(ids.NewV7()),
		Kind:           "capture_counterparty",
		Summary:        &summary,
		ProposedChange: &change,
	}
}

// A decision that blocks a customer is never folded, however many of it there
// are: each holds up somebody different, and the reader has to see each one.
func TestADecisionThatBlocksACustomerIsNeverFolded(t *testing.T) {
	staged := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		staged = append(staged, item("s"+string(rune('a'+i)), "approval", withKind("send_email")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: staged}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 5 {
		t.Fatalf("five customer-blocking decisions drew %d rows", len(got))
	}
}

// Two alike questions are not a pile. A reader answers both faster than they
// would open a group, and a "batch of 2" costs more than it saves.
func TestTwoAlikeDecisionsStayThemselves(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("a", "approval", withKind("capture_counterparty")),
			item("b", "approval", withKind("capture_counterparty")),
		},
	}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 2 {
		t.Fatalf("two decisions drew %d rows", len(got))
	}
}

// The group names a few members, so a reader can check it before answering it.
func TestABatchNamesSomeOfWhatItHolds(t *testing.T) {
	staged := []crmcontracts.AttentionItem{}
	for i := 0; i < 6; i++ {
		row := item("d"+string(rune('a'+i)), "approval", withKind("capture_counterparty"))
		title := "Is address " + string(rune('a'+i)) + " a contact?"
		row.Title = &title
		staged = append(staged, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: staged}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if got[0].Batch.Sample == nil || len(*got[0].Batch.Sample) != 3 {
		t.Fatal("the group names none of what it holds, so it cannot be checked")
	}
}

// A hundred unanswered threads is a real backlog, and a page that is nothing
// but them tells a rep their day holds no deals, tasks or decisions. The
// longest-waiting few lead; the rest are demoted, never dropped.
func TestOneKindOfWorkCannotTakeTheWholePage(t *testing.T) {
	waiting := []WaitingCustomer{}
	for i := 0; i < 30; i++ {
		waiting = append(waiting, WaitingCustomer{
			ActivityID: ids.NewV7(),
			Subject:    "Thread " + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Since:      rankInstant.Add(-time.Duration(30-i) * 24 * time.Hour),
		})
	}
	day := crmcontracts.Attention{
		AsOf:    rankInstant,
		Planned: []crmcontracts.AttentionItem{item("task", "task", withDue(rankInstant.Add(-time.Hour)))},
	}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 100, waiting)

	// Every wait is still on the page, and every one still SAYS it is a wait.
	// Rewriting the level of the ninth would tell the reader it was agreed work
	// while its own row went on saying a buyer wrote last.
	kept := 0
	for _, row := range out.Queue {
		if row.Source == "customer_waiting" {
			kept++
			if row.Level != levelWaiting {
				t.Fatalf("a waiting customer was relabelled as level %d", row.Level)
			}
		}
	}
	if kept != 30 {
		t.Fatalf("thirty waits produced %d rows — some were dropped", kept)
	}
	// What changes is only the ORDER: the overdue task is reachable rather than
	// buried under the whole backlog.
	for i, row := range out.Queue {
		if row.Source == "task" && i > waitingLead {
			t.Fatalf("the overdue task sat at position %d, below the whole backlog", i+1)
		}
	}
}

// The concept's clearest complaint: the product knew the buyer had written,
// knew nobody had answered, and knew the answer was to draft a reply — and the
// page said only "no contact for 83 days".
func TestAWaitingRowNamesTheMessageAReplyWouldAnswer(t *testing.T) {
	message := ids.NewV7()
	waiting := []WaitingCustomer{{
		ActivityID: message,
		Subject:    "Re: pricing",
		Since:      rankInstant.Add(-83 * 24 * time.Hour),
	}}

	out := (&Service{}).worklistFrom(context.Background(),
		crmcontracts.Attention{AsOf: rankInstant}, scopeAll, "", 25, waiting)

	move := out.Queue[0].Move
	if move == nil {
		t.Fatal("the row reports a wait and names no next step")
	}
	if move.Action != "draft_reply" {
		t.Fatalf("the move is %q, wanted the reply", move.Action)
	}
	if ids.UUID(move.ActivityId) != message {
		t.Fatal("the move names the wrong message, so a reply would answer the wrong thing")
	}
}

// And the deal's money rides onto the row that absorbed it. A reader told a
// customer is waiting still wants to know the deal is worth six figures.
func TestAnAbsorbedDealKeepsItsMoneyOnTheWaitingRow(t *testing.T) {
	deal := ids.NewV7()
	amount := int64(160_100_00)
	dealID := openapi_types.UUID(deal)
	at := []crmcontracts.AttentionItem{{
		Id:      deal.String(),
		Source:  "deal_at_risk",
		Subject: &crmcontracts.AttentionSubject{Type: "deal", Id: dealID},
		Deal:    &crmcontracts.AttentionDealFacts{AmountMinor: &amount},
		Actions: []crmcontracts.AttentionItemActions{},
	}}
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: &at}
	waiting := []WaitingCustomer{{
		ActivityID: ids.NewV7(),
		Since:      rankInstant.Add(-3 * 24 * time.Hour),
		DealID:     deal,
	}}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, waiting)

	if len(out.Queue) != 1 {
		t.Fatalf("one message about one deal produced %d rows", len(out.Queue))
	}
	if out.Queue[0].Deal == nil || out.Queue[0].Deal.AmountMinor == nil {
		t.Fatal("the deal's money was lost when its row was absorbed")
	}
	if *out.Queue[0].Deal.AmountMinor != amount {
		t.Fatalf("the row says %d, wanted the deal's own amount", *out.Queue[0].Deal.AmountMinor)
	}
}

// Eight rows saying a recap did not generate are ONE thing that is broken.
// Repeating it eight times is aggregation failure rather than urgency — the
// concept's words, and the real workspace holds 163 failures of one AI task.
func TestAlikeSystemFailuresAreOneIncident(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 8; i++ {
		failures = append(failures, aiFailure(i, "site_triage"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 1 {
		t.Fatalf("eight failures of one task drew %d rows", len(got))
	}
	if got[0].Batch == nil || got[0].Batch.Count != 8 {
		t.Fatal("the incident does not say how many times it happened")
	}
	if got[0].Batch.Cause == nil || *got[0].Batch.Cause != "ai_work_health:site_triage" {
		t.Fatal("the incident does not name WHAT is broken")
	}
}

// Two causes are two incidents. Grouping by source alone would tell a reader
// two things are broken and name neither.
func TestTwoBrokenThingsAreTwoIncidents(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		for _, task := range []string{"site_triage", "signal_extract"} {
			failures = append(failures, aiFailure(i*2+len(task)%2, task))
		}
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 2 {
		t.Fatalf("two broken tasks drew %d rows", len(got))
	}
	causes := map[string]bool{}
	for _, row := range got {
		if row.Batch != nil && row.Batch.Cause != nil {
			causes[*row.Batch.Cause] = true
		}
	}
	if !causes["ai_work_health:site_triage"] || !causes["ai_work_health:signal_extract"] {
		t.Fatalf("the incidents name %v, wanted both broken tasks", causes)
	}
}

// An incident is not hygiene: while something is broken, every quiet claim on
// the page is suspect, so it keeps its own band rather than being filed with
// the routine tidying.
func TestAnIncidentIsNotFiledAsRoutineTidying(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		failures = append(failures, item("m"+string(rune('a'+i)), "capture_health", withKind("reauth_required")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if got[0].Category != "system" {
		t.Fatalf("the incident is filed under %q, wanted system", got[0].Category)
	}
	if got[0].Consequence == "data_drifts" {
		t.Fatal("a broken mailbox says the records drift, which is not what it costs")
	}
}

// A bounced email is a customer CONSEQUENCE, not a system condition. Three
// bounces are three customers who did not get their message, and folding them
// by the provider's reason would hide two of them.
func TestBouncesAreNeverFoldedIntoAnIncident(t *testing.T) {
	bounces := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		bounces = append(bounces, item("b"+string(rune('a'+i)), "bounce", withKind("hard")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Bounces: &bounces}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 4 {
		t.Fatalf("four bounced customers drew %d rows — some were hidden", len(got))
	}
}

// WHICH field names the cause depends on the producer. An AI run's title is
// its own summary, written per run, so grouping on it would draw a hundred and
// sixty-three incidents for one broken task — the workspace's real number.
func TestAIFailuresGroupByWhatRanNotByEachRunsOwnWords(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		row := aiFailure(i, "site_triage")
		summary := "reading acme" + string(rune('a'+i)) + ".com failed"
		row.Title = &summary
		failures = append(failures, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 1 {
		t.Fatalf("five failures of one task drew %d incidents", len(got))
	}
	if got[0].Batch.Cause == nil || *got[0].Batch.Cause != "ai_work_health:site_triage" {
		t.Fatalf("the incident names %v, wanted the task that broke", got[0].Batch.Cause)
	}
}

// Two broken mailboxes are two things to reconnect. A heading that says
// "disconnected" once sends the reader to fix one and silently loses the other.
func TestTwoBrokenMailboxesAreTwoIncidents(t *testing.T) {
	rows := []crmcontracts.AttentionItem{}
	for _, mailbox := range []string{"sales@acme.test", "lena@acme.test"} {
		for i := 0; i < 4; i++ {
			row := item(mailbox+string(rune('a'+i)), "capture_health", withKind("disconnected"))
			cause := "capture_health:disconnected:" + mailbox
			row.CauseRef = &cause
			rows = append(rows, row)
		}
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &rows}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 2 {
		t.Fatalf("two broken mailboxes drew %d rows, wanted one incident each", len(got))
	}
}

// Capture and overlay sync both name a condition `sync_failing`. Grouping on
// the condition word alone merges two unrelated failures under one heading
// that names neither.
func TestTwoSourcesSharingAConditionWordAreNotOneIncident(t *testing.T) {
	capture := []crmcontracts.AttentionItem{}
	for i := 0; i < 3; i++ {
		row := item("c"+string(rune('a'+i)), "capture_health", withKind("sync_failing"))
		cause := "capture_health:sync_failing:sales@acme.test"
		row.CauseRef = &cause
		capture = append(capture, row)
	}
	ai := []crmcontracts.AttentionItem{}
	for i := 0; i < 3; i++ {
		row := item("a"+string(rune('a'+i)), "ai_work_health", withKind("sync_failing"))
		cause := "ai_work_health:sync_failing"
		row.CauseRef = &cause
		ai = append(ai, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &capture, AiWorkHealth: &ai}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 2 {
		t.Fatalf("two sources sharing a condition word drew %d rows, wanted one each", len(got))
	}
}

// A row that names no condition never groups. An ungrouped row is one row too
// many; a wrongly grouped one hides a failure the reader never learns about.
func TestASystemRowWithNoNamedConditionNeverGroups(t *testing.T) {
	rows := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		rows = append(rows, item("s"+string(rune('a'+i)), "capture_health", withKind("disconnected")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &rows}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant)))

	if len(got) != 5 {
		t.Fatalf("rows naming no condition folded into %d — a failure was hidden", len(got))
	}
}

// aiFailure builds a troubled-AI row through the PRODUCTION renderer, so a test
// about grouping is a test about how the product derives the grouping key. A
// test that hand-set the marker would pass while the renderer set nothing.
func aiFailure(seq int, taskKind string) crmcontracts.AttentionItem {
	return aiWorkItem(TroubledRun{
		ID:         ids.MustParse(fmt.Sprintf("01a05500-0000-7000-8000-0000000%05x", seq)),
		Kind:       taskKind,
		State:      "failed",
		OccurredAt: rankInstant,
	})
}

// A lane the feed renders and the queue never reads is a lane nobody sees: the
// rows are the page, and an item that reaches no row reaches no reader. This is
// the arm that would have caught an undelivered card rendered into a lane the
// classifier walked straight past.
func TestAGivenUpSendReachesTheQueueAndSaysWhatItCost(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:        rankInstant,
		Undelivered: lane(item("u1", "undelivered")),
		Bounces:     lane(item("b1", "bounce")),
	}

	rows := classifyDay(day, rankInstant)

	var undelivered, bounced *ranked
	for i := range rows {
		switch rows[i].item.Id {
		case "u1":
			undelivered = &rows[i]
		case "b1":
			bounced = &rows[i]
		}
	}
	if undelivered == nil {
		t.Fatalf("the undelivered lane produced no row; the queue carries %d row(s) and none of them is the send", len(rows))
	}
	// Not the bounce's consequence: nobody received this one because nobody
	// was ever sent it, and the reader's move is to send it rather than to
	// fix an address.
	if undelivered.item.Consequence != crmcontracts.WorklistItemConsequence("you_believe_it_happened") {
		t.Errorf("a send that never left says %q, want you_believe_it_happened", undelivered.item.Consequence)
	}
	if bounced == nil || bounced.item.Consequence == undelivered.item.Consequence {
		t.Errorf("the two send failures read alike (%v), and they are not the same news", bounced)
	}
}
