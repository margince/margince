// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The lead first-response SLA (formulas §18): the deterministic clock, what
// counts as a first response, and the at-most-once breach scan. The target
// is the §18 default; the RC-5 per-workspace override is not wired here yet.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadSLAClock is the clock the derived SLA fields are read against. A
// package variable rather than a Store field because the scanner that
// derives them has no store in hand; tests pin it.
var leadSLAClock = time.Now

// leadSLAFields derives the wire's sla_deadline_at and sla_state from the
// stored clock start, first response and closure (formulas §18.1) under the
// installation's policy. A closed or answered lead owes nothing and reads
// null; with the target switched off every lead does.
func leadSLAFields(policy leadSLAPolicy, routedAt *time.Time, createdAt time.Time, firstResponseAt, archivedAt *time.Time) (*time.Time, *crmcontracts.LeadSlaState) {
	if !policy.enabled || archivedAt != nil {
		return nil, nil
	}
	start := createdAt
	if routedAt != nil {
		start = *routedAt
	}
	deadline := start.Add(policy.target)
	if firstResponseAt != nil {
		return &deadline, nil
	}
	now := leadSLAClock().UTC()
	state := crmcontracts.LeadSlaStateWithinTarget
	switch {
	case now.After(deadline):
		state = crmcontracts.LeadSlaStateBreached
	case deadline.Sub(now) <= policy.atRisk():
		state = crmcontracts.LeadSlaStateAtRisk
	}
	return &deadline, &state
}

// slaStateClause renders one sla_state filter as SQL over the lead's own
// columns, with the same arithmetic leadSLAFields applies in Go: the list
// and the row must agree about which leads are overdue.
//
// The instant is the application clock, bound as a parameter, never the
// database's now(): the row's sla_state is derived against leadSLAClock, and
// a filter reading a different clock — the container's, seconds adrift, or
// the other side of a boundary crossed mid-request — would return an at_risk
// row whose own payload says breached.
//
// With the target switched off no lead is in any SLA state, so the filter
// matches nothing rather than pretending a default target.
// leadOwesAReplySQL is the one spelling of "this lead still owes a first
// reply": live, and nobody has answered it.
//
// FOUR readers ask it — the SLA state filter, the breach scan, the work
// queue's band, and the list's own unanswered dial — and the question is one.
// Spelled separately they drift, and a queue that disagrees with the filter
// feeding it reports a count nobody can reconcile.
//
// Deliberately NOT a statement about the status ladder. A lead the system moved
// to `contacted` because a cold outbound went out has had no genuine response,
// and §18.1 is explicit that an auto-touch does not satisfy first response — so
// the rung a lead sits on says nothing about whether somebody replied to it.
//
// Not a statement about OWNERSHIP either. An unowned lead is the funnel's normal
// arrival state (CreateLead assigns nobody unless a human names an owner), so a
// queue admitting only owned rows would drop the whole unassigned backlog the
// Unassigned dial exists to show. Only the BREACH SCAN narrows that way, and it
// does so in its own statement: escalating a row nobody has taken on stamps
// sla_breached_at and suppresses the real escalation once somebody does.
//
// Held by: TestTheOwesAReplyPredicateHasOneSpelling (leadowespelling_test.go)
const leadOwesAReplySQL = "archived_at IS NULL AND first_response_at IS NULL"

func slaStateClause(policy leadSLAPolicy, state crmcontracts.ListLeadsParamsSlaState, arg func(any) int) string {
	if !policy.enabled {
		return "FALSE"
	}
	deadline := "COALESCE(routed_at, created_at) + $%d * interval '1 minute'"
	open := leadOwesAReplySQL + " AND "
	minutes := policy.targetMinutes()
	now := leadSLAClock().UTC()
	switch crmcontracts.LeadSlaState(state) {
	case crmcontracts.LeadSlaStateBreached:
		return storekit.SQLf(open+deadline+" < $%d", arg(minutes), arg(now))
	case crmcontracts.LeadSlaStateAtRisk:
		return storekit.SQLf(open+deadline+" >= $%d AND "+deadline+" - $%d * interval '1 minute' <= $%d",
			arg(minutes), arg(now), arg(minutes), arg(int(policy.atRisk()/time.Minute)), arg(now))
	default:
		return storekit.SQLf(open+deadline+" - $%d * interval '1 minute' > $%d",
			arg(minutes), arg(int(policy.atRisk()/time.Minute)), arg(now))
	}
}

// firstResponseColumn is the lead's §18.1 first-response stamp.
const firstResponseColumn = "first_response_at"

// slaBreachedColumn is the lead's at-most-once §18.2 breach mark.
const slaBreachedColumn = "sla_breached_at"

// firstResponseSet is the SET fragment every disposition write carries: the
// first genuine response is recorded once and never moved.
const firstResponseSet = firstResponseColumn + ` = COALESCE(` + firstResponseColumn + `, now())`

// SLABreach is one lead whose first-response deadline passed unanswered on
// this scan — what the escalation acts on.
//
// Name is what the lead is called, and the SELECT works it out the way
// leadIdentityName does: a full_name that is present and empty is not a name,
// so the address behind it is. A bare COALESCE would answer the empty string,
// and the escalation would page an owner about a lead it could not name while
// the promotion of the same lead names it by its address.
type SLABreach struct {
	LeadID   ids.LeadID
	OwnerID  *ids.UserID
	Deadline time.Time
	Name     string
}

// RecordLeadFirstResponse stamps the lead's first genuine response from an
// outbound activity (formulas §18.1) — the one first-response trigger that
// does not ride another lead write. It answers whether this call was the
// one that set it, so the caller can tell a real change from a replay.
func (s *Store) RecordLeadFirstResponse(ctx context.Context, leadID ids.LeadID, at time.Time) (bool, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return false, err
	}
	set := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		set, err = recordFirstResponseTx(ctx, tx, leadID, at)
		return err
	})
	return set, err
}

// recordFirstResponseTx is RecordLeadFirstResponse's body, inside a
// transaction the caller already holds.
//
// Two callers, one spelling: the outbox subscriber opens its own transaction
// for it, and the breach scan calls it while holding the lead row it is about
// to judge. A second implementation there would be a second answer to "what
// counts as the first response" and to "when is a stamp a replay", in the one
// place where getting either wrong marks a lead that was answered.
func recordFirstResponseTx(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, at time.Time) (bool, error) {
	set := false
	err := func() error {
		// The outbox subscriber that drives this is unbounded, so the probe is a
		// no-op today; it is here so the write carries its own scope the day a
		// human-facing caller stamps a first response.
		if err := auth.EnsureWritableLive(ctx, tx, "lead", leadID.UUID); err != nil {
			return err
		}
		lock, err := storekit.LockRow(ctx, tx, "lead", leadID.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		var current *time.Time
		if err := tx.QueryRow(ctx, `SELECT first_response_at FROM lead WHERE id = $1`, leadID).Scan(&current); err != nil {
			return err
		}
		// The FIRST response is the earliest one, not the first one this
		// subscriber happened to process: the bus is at-least-once and
		// unordered, so a 09:00 reply may arrive after a 10:00 one. A later
		// or equal stamp on a lead already answered is a replay and a no-op.
		if current != nil && !at.Before(*current) {
			return nil
		}
		p := storekit.NewPatch()
		p.Set(firstResponseColumn, current, at)
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "lead", leadID.UUID,
			map[string]any{firstResponseColumn: current}, map[string]any{firstResponseColumn: at})
		if err != nil {
			return err
		}
		set = true
		return storekit.EmitEvent(ctx, tx, auditID, leadID.UUID, crmcontracts.PublicEventLeadUpdated{
			ChangedFields: map[string]any{eventKeyDelta: map[string]any{firstResponseColumn: at}},
		})
	}()
	return set, err
}

// isFirstResponseActivity decides whether a captured activity is a genuine
// response (formulas §18.1) rather than a cold-outbound auto-touch: a
// human's outbound always is; an agent's counts only when the lead had
// already written in — a touch with nothing to respond to is the
// anti-pollution case §2 names. A note a rep typed into the composer
// (humanLoggedNote) counts too, and for the same reason it walks the
// ladder: it records outreach that happened off-system, and a lead the
// stepper shows as Contacted must not go on to breach the first-response
// target as if nobody had answered.
func isFirstResponseActivity(t leadResponseTouch) bool {
	if humanLoggedNote(t) {
		return true
	}
	if t.direction != "outbound" {
		return false
	}
	if humanCaptured(t) {
		return true
	}
	return t.hadInbound
}

// answeredBy is the ground truth the breach scan judges on: the earliest
// genuine first response linked to this lead that happened at or before the
// deadline, or nil when there is none.
//
// It reads the ACTIVITIES rather than lead.first_response_at, because that
// column is a projection an event subscriber writes and the scan's whole
// hazard is running in the window before it catches up. What it must not be is
// a second definition of "genuine response": the touches come back in the same
// shape the subscriber judges, and isFirstResponseActivity — the §18.1 rule
// itself — decides each of them.
//
// EARLIEST, not any: the stamp this feeds is the first response, and a lead
// answered twice before its deadline must record the first of the two, exactly
// as the subscriber does when the bus delivers them out of order.
func answeredBy(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, deadline time.Time) (*time.Time, error) {
	touches, err := leadTouchesFor(ctx, tx, leadID, deadline)
	if err != nil {
		return nil, err
	}
	var earliest *time.Time
	for _, t := range touches {
		if !isFirstResponseActivity(t) {
			continue
		}
		if earliest == nil || t.occurredAt.Before(*earliest) {
			at := t.occurredAt
			earliest = &at
		}
	}
	return earliest, nil
}

// leadTouchesFor answers this lead's activities up to the deadline in the shape
// isFirstResponseActivity reads.
//
// The same columns and the same hadInbound sub-select as leadResponseTouches,
// asked from the other end: that one starts from ONE activity and finds the
// leads it is linked to, which is what an event subscriber knows; this starts
// from one LEAD and finds its activities, which is what a scan knows. Neither
// can be expressed as the other without asking the database for rows its
// caller has no use for.
func leadTouchesFor(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, deadline time.Time) ([]leadResponseTouch, error) {
	rows, err := tx.Query(ctx, `
		SELECT coalesce(a.direction, ''), a.captured_by, a.occurred_at,
		       EXISTS (SELECT 1 FROM activity_link li JOIN activity ai ON ai.id = li.activity_id
		               WHERE li.lead_id = $1 AND ai.direction = 'inbound'
		                 AND ai.archived_at IS NULL AND `+auth.ActivityAvailableClause("ai")+`
		                 AND ai.occurred_at < a.occurred_at),
		       a.kind, coalesce(a.meeting_status, ''), a.source
		FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.lead_id = $1 AND a.archived_at IS NULL AND a.occurred_at <= $2
		  AND `+auth.ActivityAvailableClause("a"), leadID, deadline)
	if err != nil {
		return nil, fmt.Errorf("read the lead's activities before its deadline: %w", err)
	}
	defer rows.Close()
	var out []leadResponseTouch
	for rows.Next() {
		t := leadResponseTouch{lead: leadID}
		if err := rows.Scan(&t.direction, &t.capturedBy, &t.occurredAt, &t.hadInbound,
			&t.kind, &t.meetingStatus, &t.source); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// unassignedEscalationTarget reads the configured intake seat.
//
// Answers (id, true) when an installation has named a seat that can still work
// the queue, and (zero, false) for every way it has not: no setting, an empty
// one, an unparseable one, or a seat that has since been suspended, archived,
// turned into an agent or dropped to a read seat. A seat nobody reads is the
// same silence as no seat at all, wearing a configuration that looks correct.
//
// Read through settings.ApplyTx, the ungated seam machinery applies a posture
// from, the way loadLeadSLAPolicy reads its own pair: this runs inside the SLA
// sweep under the system principal, and the value is an input to a decision
// already the sweep's to make. Asking the object gate would be asking a
// question that cannot answer no — auth.Require returns nil for
// PrincipalSystem before permissions are consulted.
//
// ApplyTx rather than the raw statement this was: it refuses any entry not
// declared MachineryApplied at Define time, so the licence to read this one
// ungated is checked rather than agreed. A decode failure is ApplyTx's to
// report, and it reports it as an error — which is the answer this path needs.
// The setting is written through a validated entry, so a value that will not
// decode means somebody wrote the row around it, and answering "nobody is
// configured" would hide that behind a queue quietly escalating to no one.
func unassignedEscalationTarget(ctx context.Context, tx pgx.Tx) (ids.UUID, bool, error) {
	configured, err := settings.ApplyTx(ctx, tx, UnassignedEscalationUserID)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("load the unassigned escalation seat: %w", err)
	}
	if configured == "" {
		// Empty IS the answer: the documented way to say no seat answers.
		return ids.UUID{}, false, nil
	}
	id, err := ids.Parse(configured)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("the unassigned escalation seat is not a user id: %w", err)
	}
	// auth.EnsureAssignee, not a query of our own: "may this seat be handed
	// work" already has one spelling, and a third reading of it is a third
	// answer. Its SCOPE half is vacuous here — the sweep runs as the system
	// principal — and its eligibility half is exactly the question.
	if err := auth.EnsureAssignee(ctx, tx, id); err != nil {
		if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			return ids.UUID{}, false, nil
		}
		return ids.UUID{}, false, fmt.Errorf("check the unassigned escalation seat: %w", err)
	}
	return id, true, nil
}
