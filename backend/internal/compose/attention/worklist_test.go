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
