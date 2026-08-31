// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What each item IS, in the queue's own terms: which band it belongs to, why it
// is there, and what happens if the reader does nothing.
//
// This is the editorial layer the lane feed does not have. A lane says "this
// came from the bounce reader"; a queue has to say "a customer never received
// your quote, and nobody else is going to notice".
//
// The consequence is derived per ITEM rather than per source, because one source
// has several honest answers: a deal past its close date SLIPS, while one merely
// idle DRIFTS, and a reader who is told the wrong one stops believing the
// right ones.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// sourceWaiting names the who-is-waiting producer. A named constant rather
// than the literal at each site: the classifier, the dedupe and the
// source-unavailable report all reach for it, and a typo in any of them would
// produce a lane nothing joins up — silently, because each half would still
// compile.
const sourceWaiting = "customer_waiting"

// subjectDeal is the subject type a deal-shaped row names.
const subjectDeal = "deal"

// classifyDay turns the assembled lanes into ranked candidates.
//
// Order of appearance does not matter — rankAll decides the order — so the lanes
// are walked in whatever order reads clearest here.
func classifyDay(day crmcontracts.Attention, asOf time.Time) []ranked {
	rows := make([]ranked, 0, 64)
	bar := materialBarOf(day)
	rows = appendLane(rows, day.Meetings, asOf, classifyMeeting)
	rows = appendLane(rows, &day.ThisMorning, asOf, classifyBriefItem)
	rows = appendLane(rows, day.Commitments, asOf, classifyCommitment)
	rows = appendLane(rows, day.DidNotRun, asOf, classifyFailedApproval)
	rows = appendLane(rows, day.Dsr, asOf, classifyDSR)
	rows = appendLane(rows, day.AtRisk, asOf, func(item crmcontracts.AttentionItem, at time.Time) ranked {
		return classifyRisk(item, at, bar)
	})
	rows = appendLane(rows, &day.Planned, asOf, classifyTask)
	rows = appendLane(rows, day.Bounces, asOf, classifyBounce)
	rows = appendLane(rows, &day.NeedsYou, asOf, classifyDecision)
	rows = appendLane(rows, day.RelationshipDecay, asOf, classifyDecay)
	rows = appendLane(rows, day.CaptureHealth, asOf, classifySystem)
	rows = appendLane(rows, day.AiWorkHealth, asOf, classifySystem)
	rows = appendLane(rows, day.AutomationHealth, asOf, classifySystem)
	rows = appendLane(rows, day.SyncHealth, asOf, classifySystem)
	rows = appendLane(rows, day.Notices, asOf, classifySystem)
	return rows
}

// appendLane classifies one lane, skipping a lane the feed did not serve.
func appendLane(
	rows []ranked,
	lane *[]crmcontracts.AttentionItem,
	asOf time.Time,
	classify func(crmcontracts.AttentionItem, time.Time) ranked,
) []ranked {
	if lane == nil {
		return rows
	}
	for _, item := range *lane {
		rows = append(rows, classify(item, asOf))
	}
	return rows
}

// base carries across every fact the two feeds share, so a field the lane feed
// already resolved — a title, a subject, an overdue flag — is never re-derived
// here and never allowed to disagree with the card the same item draws there.
func base(
	item crmcontracts.AttentionItem,
	level int,
	category crmcontracts.WorklistItemCategory,
	consequence crmcontracts.WorklistItemConsequence,
) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{
		Id:          item.Id,
		Source:      crmcontracts.WorklistItemSource(item.Source),
		Category:    category,
		Level:       level,
		Consequence: consequence,
		Kind:        item.Kind,
		Title:       item.Title,
		Detail:      item.Detail,
		Subject:     item.Subject,
		Deal:        dealFactsOf(item),
		DueAt:       item.DueAt,
		Overdue:     item.Overdue,
		OccurredAt:  item.OccurredAt,
		Actions:     carriedActions(item.Actions),
		Because:     []crmcontracts.WorklistReason{},
	}
}

// carriedActions passes the lane feed's verbs through unchanged. The queue adds
// no authority of its own: every verb still routes to the endpoint that owns it.
func carriedActions(actions []crmcontracts.AttentionItemActions) []crmcontracts.WorklistItemActions {
	out := make([]crmcontracts.WorklistItemActions, 0, len(actions))
	for _, action := range actions {
		out = append(out, crmcontracts.WorklistItemActions(action))
	}
	return out
}

// classifyMeeting: a meeting starting within the horizon is the most urgent
// thing on the page, because it happens whether or not the reader acts.
func classifyMeeting(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	level := levelAgreed
	reasons := []crmcontracts.WorklistReason{}
	if item.DueAt != nil && item.DueAt.Sub(asOf) <= meetingHorizon {
		level = levelWaiting
		reasons = append(reasons, reason("meeting_soon", nil))
	}
	row := base(item, level, "meetings", "meeting_unprepared")
	// A meeting's start time IS a deadline the reader is racing, so it counts
	// as work due — unlike a proposal's expiry, which merely lapses.
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = reasons
	return ranked{item: row, deadlineAt: deadlineOf(item.DueAt), occurredAt: occurredOf(item, asOf)}
}

// classifyCommitment: a promise the rep made. Level 2 whether or not it is
// overdue — the promise is the fact, and the date only orders it.
func classifyCommitment(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "tasks", "promise_breaks")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("promised", nil)}
	if overdueAt(item.DueAt, asOf) {
		row.Because = append(row.Because, reason("overdue", nil))
	}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyFailedApproval: the rep pressed Accept and believes it happened. That
// belief is the damage, which is why it sits beside a broken promise rather
// than with the other system news.
func classifyFailedApproval(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "you_believe_it_happened")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifyDSR: a clock the law started. It reaches only privacy admins — the
// lane is absent for everyone else — so it never needs explaining to a rep.
func classifyDSR(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelWaiting, "system", "legal_deadline_missed")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("legal_deadline", nil)}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyRisk: a deal drifting. Whether it is worth interrupting the day for
// is decided against the pipeline's own median rather than a number somebody
// typed once, so "material" tracks the business as it changes.
func classifyRisk(item crmcontracts.AttentionItem, asOf time.Time, bar materialBar) ranked {
	consequence := crmcontracts.WorklistItemConsequence("deal_drifts")
	if item.Kind != nil && *item.Kind == "close_overdue" {
		consequence = "deal_slips_past_close"
	}
	expected, known := expectedRevenue(item)
	// Material revenue interrupts the day; a smaller deal drifting is agreed
	// work like any other. The bar is the pipeline's own median rather than a
	// number somebody typed once, so "material" tracks the business as it
	// moves — and a deal whose value nobody recorded is not assumed large.
	level := levelAgreed
	if known && bar.material(expected) {
		level = levelMaterialRisk
	}
	row := base(item, level, "deals_at_risk", consequence)
	if level == levelMaterialRisk {
		row.Because = append(row.Because, reason("material", moneyOf(expected)))
	} else if known {
		row.Because = append(row.Because, reason("below_material", moneyOf(expected)))
	}
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	// The close date is a deadline the customer agreed to, so it ranks like
	// one. Without this the risk lane compared on idle days alone, and a deal
	// already past its date lost to one merely quiet for longer.
	if item.DueAt != nil {
		row.Because = append(row.Because, reason("closing_soon", nil))
	}
	return ranked{
		item:         row,
		deadlineAt:   deadlineOf(item.DueAt),
		overdue:      item.Overdue != nil && *item.Overdue,
		expectedBase: expected,
		hasExpected:  known,
		waitingDays:  quiet,
		occurredAt:   occurredOf(item, asOf),
	}
}

// dealFactsOf carries the deal's own figures onto the queue row. The lane feed
// already resolved them; dropping them here would make the client read a second
// endpoint per row to draw a card this one could have completed.
func dealFactsOf(item crmcontracts.AttentionItem) *crmcontracts.WorklistDealFacts {
	if item.Deal == nil {
		return nil
	}
	return &crmcontracts.WorklistDealFacts{
		StageId:     item.Deal.StageId,
		OwnerId:     item.Deal.OwnerId,
		AmountMinor: item.Deal.AmountMinor,
		Currency:    item.Deal.Currency,
	}
}

// expectedRevenue is what the deal is worth times how likely it is to land.
//
// The win probability lives on the stage rather than the deal, and this feed
// does not read stages — so until that read exists the amount stands in for the
// expectation. Naming that here rather than silently multiplying by one: the
// figure is comparable between deals in one currency, which is what the
// ordering needs, and it will get more accurate rather than change meaning.
func expectedRevenue(item crmcontracts.AttentionItem) (int64, bool) {
	if item.Deal == nil || item.Deal.AmountMinor == nil {
		return 0, false
	}
	return *item.Deal.AmountMinor, true
}

func moneyOf(minor int64) *crmcontracts.WorklistValue {
	value := minor
	return &crmcontracts.WorklistValue{Kind: "money", Minor: &value}
}

// classifyWaiting: somebody wrote and nobody answered.
//
// Level 1, the top band below a pin, and the reason is the concept's own: a
// customer waiting on a reply is the one thing on this page where the cost of
// doing nothing falls on somebody else. It outranks a promise, a drifting deal
// and every decision.
//
// The draft verb is offered only when the message can be READ. There is no
// drafting a reply to words this reader may not see, and a button that opened
// an empty composer would be worse than no button.
func classifyWaiting(waiting WaitingCustomer, asOf time.Time) ranked {
	subject := waiting.Subject
	days := int(asOf.Sub(waiting.Since).Hours() / 24)
	if days < 0 {
		days = 0
	}
	row := crmcontracts.WorklistItem{
		Id:          waiting.ActivityID.String(),
		Source:      sourceWaiting,
		Category:    sourceWaiting,
		Level:       levelWaiting,
		Consequence: "buyer_waits",
		Because: []crmcontracts.WorklistReason{
			reason("buyer_wrote_last", nil),
			reason("waiting_days", daysValue(days)),
		},
		Actions: []crmcontracts.WorklistItemActions{},
	}
	// The subject travels because the row exists at all only for a reader the
	// content gate admitted: a message this reader may not read produces no
	// row, rather than a row with its words removed.
	if subject != "" {
		row.Title = &subject
	}
	// The record the reply would be about, most specific first: the deal a
	// thread belongs to says more than the company it is filed under.
	switch {
	case !waiting.DealID.IsZero():
		row.Subject = subjectOf(subjectDeal, waiting.DealID)
	case !waiting.PersonID.IsZero():
		row.Subject = subjectOf("person", waiting.PersonID)
	case !waiting.OrganizationID.IsZero():
		row.Subject = subjectOf("organization", waiting.OrganizationID)
	}
	if openableSubject(row.Subject) {
		row.Actions = append(row.Actions, crmcontracts.WorklistItemActions(actionOpen))
	}
	occurred := waiting.Since
	row.OccurredAt = &occurred
	// The message IS the row's id, so the move needs nothing this row does not
	// already carry. The product knew the buyer had written and knew nobody had
	// answered; naming the step is the difference between a page that reports
	// and a page a rep can work from.
	activity := openapi_types.UUID(waiting.ActivityID)
	row.Move = &crmcontracts.WorklistMove{
		Action:     "draft_reply",
		ActivityId: &activity,
	}
	return ranked{item: row, waitingDays: days, occurredAt: waiting.Since}
}

// dropDealsAlreadyWaiting removes the at-risk row for a deal somebody is
// already waiting on.
//
// One unanswered message must not become two rows. The waiting row is strictly
// the more urgent and the more actionable of the two — it names the message to
// reply to — so it wins, and the drifting row's ground rides along as a reason
// rather than as a second obligation.
func dropDealsAlreadyWaiting(rows []ranked) []ranked {
	waitingDeals := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceWaiting && row.item.Subject != nil &&
			row.item.Subject.Type == subjectDeal {
			waitingDeals[row.item.Subject.Id.String()] = true
		}
	}
	if len(waitingDeals) == 0 {
		return rows
	}
	// The deal's own facts ride onto the waiting row that absorbed it: a reader
	// told a customer is waiting still wants to know the deal is worth €160k.
	facts := map[string]*crmcontracts.WorklistDealFacts{}
	for _, row := range rows {
		if row.item.Source == "deal_at_risk" && waitingDeals[row.item.Id] {
			facts[row.item.Id] = row.item.Deal
		}
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Source == "deal_at_risk" && waitingDeals[row.item.Id] {
			continue
		}
		if row.item.Source == sourceWaiting && row.item.Subject != nil &&
			row.item.Subject.Type == subjectDeal && row.item.Deal == nil {
			row.item.Deal = facts[row.item.Subject.Id.String()]
		}
		kept = append(kept, row)
	}
	return kept
}

// classifyBriefItem: what the overnight run put at the top of the day. It is a
// suggestion about where to start rather than something waiting on the reader,
// so it sits with agreed work — but it belongs ON the queue, because a lane the
// ranking never sees is a lane the reader was told to read separately, which is
// the arrangement this endpoint exists to end.
func classifyBriefItem(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelAgreed, "deals_at_risk", "deal_drifts")
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifyTask: work already agreed. Overdue is the fact that moves it; a task
// nobody dated is real work and is not today's.
func classifyTask(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelAgreed, "tasks", "task_slips")
	stampDeadline(&row, item.DueAt, asOf)
	if overdueAt(item.DueAt, asOf) {
		row.Because = append(row.Because, reason("overdue", nil))
	} else if item.DueAt != nil {
		row.Because = append(row.Because, reason("due_today", nil))
	}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyBounce: a customer never received something the rep believes they
// sent. A consequence with a named customer and a verb, so it ranks as its own
// row rather than disappearing into an aggregate.
func classifyBounce(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "customer_never_received")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifyDecay: a relationship going quiet. Nobody is waiting on the reader
// for it, which is exactly why it goes unnoticed — and why it sits low rather
// than not at all.
func classifyDecay(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelRoutine, "system", "data_drifts")
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	return ranked{item: row, waitingDays: quiet, occurredAt: occurredOf(item, asOf)}
}

// classifySystem: the pipes. A mailbox that stopped capturing makes every quiet
// claim on this page suspect, which is why it is here at all rather than only in
// an admin screen.
func classifySystem(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	consequence := crmcontracts.WorklistItemConsequence("work_blocked")
	if item.Source == "capture_health" || item.Source == "sync_health" {
		consequence = "mailbox_blind"
	}
	if item.Source == "notice" {
		consequence = valueNone
	}
	row := base(item, levelBlocking, "system", consequence)
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// stampDeadline marks an item whose date the READER owes.
//
// The flag is what tells a real deadline from a staged proposal's expiry: the
// producers whose date is a deadline resolve it here, and the ones whose date
// is a lapse moment leave it unset, so the header can count work due without
// counting proposals that merely go stale.
func stampDeadline(row *crmcontracts.WorklistItem, due *time.Time, asOf time.Time) {
	if due == nil {
		return
	}
	past := overdueAt(due, asOf)
	row.Overdue = &past
}

func reason(kind crmcontracts.WorklistReasonKind, value *crmcontracts.WorklistValue) crmcontracts.WorklistReason {
	return crmcontracts.WorklistReason{Kind: kind, Value: value}
}

func deadlineOf(due *time.Time) time.Time {
	if due == nil {
		return time.Time{}
	}
	return *due
}

// occurredOf answers when the reported thing happened, falling back to the read
// instant so a row with no timestamp sorts as "now" rather than as 1 January
// year one — which would put every undated row at the head of its level.
func occurredOf(item crmcontracts.AttentionItem, asOf time.Time) time.Time {
	if item.OccurredAt != nil {
		return *item.OccurredAt
	}
	return asOf
}

// quietDaysOf reads the idle count the risk and decay cards carry as their
// detail. A detail that is not a number is a different kind of supporting line,
// and reading it as zero is the honest answer rather than a guess.
func quietDaysOf(item crmcontracts.AttentionItem) int {
	if item.Detail == nil {
		return 0
	}
	days := 0
	for _, r := range *item.Detail {
		if r < '0' || r > '9' {
			return 0
		}
		days = days*10 + int(r-'0')
	}
	return days
}
