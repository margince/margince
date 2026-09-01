// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The lead concern: an inbound lead nobody has replied to yet, and where it
// sits against the rest of the day.
//
// The clock is not this file's to invent. `sla_state` and `sla_deadline_at`
// are derived on every lead read (formulas §18.1), so what happens here is
// ranking an answer the people module already gave — a second opinion about
// when a reply is late would be one the lead screen could disagree with.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// sourceLeadResponse is a lead still owed its first reply.
const sourceLeadResponse = "lead_response"

// categoryLeads is its own category rather than customer_waiting.
//
// A buyer who wrote and is waiting for an answer, and a lead nobody has
// contacted at all, are different work: the first has a message to reply to,
// the second has nobody to reply to yet. Folding them together would put a
// "draft the reply" verb on a row with no thread behind it.
const categoryLeads = "leads"

// leadLead is how many overdue leads lead the page, mirroring waitingLead.
//
// A cap on how much of ONE kind a reader meets before they see the others,
// rather than a cap on the source: the rest stay ranked and reachable.
const leadLead = 8

// classifyLead ranks one lead against everything else on the day.
//
// Breached sits at levelWaiting beside a waiting customer, because it is the
// same fact from the other side: somebody outside is waiting and the clock has
// already run out. At-risk sits at levelPromise — a deadline the rep can still
// meet. A lead nobody owns and whose clock has not started is agreed work.
func classifyLead(lead OwedLead, asOf time.Time) ranked {
	level, because := leadStanding(lead, asOf)
	if lead.OwnerID.IsZero() {
		because = append(because, reason("unassigned", nil))
	}
	name := lead.Name
	row := crmcontracts.WorklistItem{
		Id:          lead.ID.String(),
		Source:      sourceLeadResponse,
		Category:    categoryLeads,
		Level:       level,
		Consequence: "buyer_waits",
		Because:     because,
		Subject:     subjectOf("lead", lead.ID),
		Actions:     []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActions(actionOpen)},
	}
	if name != "" {
		row.Title = &name
	}
	var deadline *time.Time
	if !lead.DeadlineAt.IsZero() {
		at := lead.DeadlineAt
		deadline = &at
		row.DueAt = &at
		stampDeadline(&row, &at, asOf)
	}
	return ranked{item: row, deadlineAt: deadlineOf(deadline), occurredAt: asOf}
}

// leadStanding reads the state the people module derived, and says what it
// means for the day's order.
//
// An unrecognised state ranks as agreed work rather than being dropped: the
// vocabulary is the contract's and may grow, and a lead that vanished from the
// queue because this switch had not heard of its state would be work nobody
// could see.
func leadStanding(lead OwedLead, asOf time.Time) (int, []crmcontracts.WorklistReason) {
	switch lead.State {
	case string(crmcontracts.LeadSlaStateBreached):
		because := []crmcontracts.WorklistReason{reason("response_overdue", nil)}
		if !lead.DeadlineAt.IsZero() {
			days := int(asOf.Sub(lead.DeadlineAt).Hours() / 24)
			if days > 0 {
				because = append(because, reason("waiting_days", daysValue(days)))
			}
		}
		return levelWaiting, because
	case string(crmcontracts.LeadSlaStateAtRisk):
		return levelPromise, []crmcontracts.WorklistReason{reason("response_due_soon", nil)}
	default:
		return levelAgreed, []crmcontracts.WorklistReason{}
	}
}

// dropEscalationTasksAlreadyOwed removes the task the SLA escalation raised for
// a lead this queue is already showing.
//
// The escalation writes a task AND a notice when a lead breaches, and both were
// already reaching the page before this lane existed. Adding the lead itself
// would make one late reply three rows: the lead, the task about the lead, and
// a notice about the same thing. The lead row is the one that survives — it
// carries the deadline and the state, and its verb opens the record the other
// two only describe.
//
// Matched on the LINK rather than on a source system the wire does not carry: a
// task filed under this lead, while the lead is on the page owing a reply, is
// about that reply. The notice is left alone — it is read-once and personal,
// and it names no lead to match on.
func dropEscalationTasksAlreadyOwed(rows []ranked) []ranked {
	owed := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceLeadResponse && row.item.Subject != nil {
			owed[row.item.Subject.Id.String()] = true
		}
	}
	if len(owed) == 0 {
		return rows
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Source == sourceTask && row.item.Subject != nil &&
			row.item.Subject.Type == "lead" && owed[row.item.Subject.Id.String()] {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}
