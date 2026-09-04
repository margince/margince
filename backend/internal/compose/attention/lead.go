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
	"context"
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LeadResponses is the inbound leads still owed a first reply.
//
// The SDR's half of the morning. A routed lead nobody has answered goes cold on
// a clock the product already runs — sla_deadline_at and sla_state are derived
// on every lead read — and until now that clock ticked on a screen of its own
// while the queue that claims to be "what should I do next" said nothing about
// it.
//
// `tracked` is the installation's first-response policy, and false is not an
// empty list: with the target switched off no lead owes a reply at a stated
// time, so the lane renders ABSENT rather than empty. Saying "no leads are
// overdue" where nothing measures overdue would be a claim the product cannot
// support.
type LeadResponses interface {
	Owed(ctx context.Context, scope TaskScope, owner ids.UUID, limit int) (rows []OwedLead, tracked bool, err error)
}

// OwedLead is one inbound lead nobody has replied to yet.
type OwedLead struct {
	ID   ids.UUID
	Name string
	// OwnerID is zero when the lead is assigned to nobody, which is its own
	// kind of urgency rather than a missing field.
	OwnerID ids.UUID
	// DeadlineAt is when the first reply was owed. Zero when the policy states
	// no target, in which case State is empty too.
	DeadlineAt time.Time
	// State is the contract's own vocabulary: within_target, at_risk, breached.
	State string
}

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
		Subject:     subjectOf(string(subjectLead), lead.ID),
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
	return ranked{
		item: row, deadlineAt: deadlineOf(deadline), occurredAt: asOf,
		// Who owes the first reply, so the scope filters can judge this row the
		// way they judge every other.
		//
		// keepOwnedBy used to carry a case for this source, because it read only
		// the DEAL's owner and a lead row carries a lead subject and no deal —
		// so without the case every lead was dropped from a named owner's queue.
		// Saying who owns the row is the general answer, and it costs the next
		// source no case of its own.
		owner: lead.OwnerID,
		// And the same answer for the client. A lead nobody has taken is the
		// state this lane exists to surface, so a zero id here means it.
		ownerRef: ownerFrom(lead.OwnerID),
	}
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
			days := daysSince(lead.DeadlineAt, asOf)
			if days > 0 {
				because = append(because, reason("waiting_days", daysValue(days)))
			}
		}
		return levelWaiting, because
	case string(crmcontracts.LeadSlaStateAtRisk):
		// The deadline travels with the reason, because "reply due soon" alone
		// asks the rep to guess how soon. Its breached sibling above already
		// carries a figure (the days it has been overdue); this is the same
		// courtesy on the side of the clock where it can still be met.
		//
		// A row whose deadline is missing says the plain sentence rather than a
		// zero time: absent is honest, 1 January 1 is not.
		if lead.DeadlineAt.IsZero() {
			return levelPromise, []crmcontracts.WorklistReason{reason("response_due_soon", nil)}
		}
		return levelPromise, []crmcontracts.WorklistReason{
			reason("response_due_soon", deadlineValue(lead.DeadlineAt)),
		}
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
			row.item.Subject.Type == subjectLead && owed[row.item.Subject.Id.String()] {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// leadRead is what one lead-response read produced: the rows, and whether the
// read HAPPENED at all.
//
// The two travel together because `bounded` is only meaningful when `read` is
// true — an unread source is absent from the page, and an absent source has no
// bound to report. Held apart as two booleans they could be set independently,
// and the combination that says "not read, but bounded" would publish a reach
// row for a source the page never consulted.
type leadRead struct {
	rows []OwedLead
	// read is false when no lead source is bound, when the installation
	// measures no first response, or when the read was refused or failed.
	read bool
}

// bounded reports whether the read stopped at its cap, so the page can say
// there is more than it is showing. Only ever asked of a read that happened —
// an unread source is absent from the page and has no bound to report.
func (l leadRead) bounded() bool {
	return len(l.rows) >= leadResponseBound
}

// owedLeads reads the leads still owed a first reply, or names why it could not.
//
// An installation with no first-response target has no leads that are LATE, so
// the source is absent from the page entirely. Reporting zero overdue leads
// where nothing measures overdue would be a number the product cannot stand
// behind.
func (s *Service) owedLeads(
	ctx context.Context,
) (leadRead, *crmcontracts.WorklistSourceUnavailable) {
	if s.leads == nil {
		return leadRead{}, nil
	}
	owed, tracked, err := s.leads.Owed(ctx, s.taskScope, s.taskOwner, leadResponseBound)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return leadRead{}, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceLeadResponse, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		}
	case err != nil:
		slog.ErrorContext(ctx, "the leads-owed-a-reply read failed", "error", err)
		return leadRead{}, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceLeadResponse, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	case !tracked:
		return leadRead{}, nil
	default:
		return leadRead{rows: owed, read: true}, nil
	}
}
