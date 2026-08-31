// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the queue says about a day, as opposed to how it orders one.
//
// The projection's promise: every item states what happens if the reader does
// nothing, the figures above the list match the rows below it, and a day never
// reads as clear when something that would have filled it was never read.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	if got[0].Source != "capture_health" || got[0].Reason != crmcontracts.Withheld {
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

	out, err := (&Service{}).worklistFrom(day, "", 3)
	if err != nil {
		t.Fatalf("assembling the page: %v", err)
	}

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
