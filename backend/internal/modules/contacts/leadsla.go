// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The lead first-response SLA (formulas §18): the deterministic clock, what
// counts as a first response, and the at-most-once breach scan. The target
// is the §18 default; the RC-5 per-workspace override is not wired here yet.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadSLAClock is the clock the derived SLA fields are read against. A
// package variable rather than a Store field because the scanner that
// derives them has no store in hand; tests pin it.
var leadSLAClock = time.Now

// slaNowKey carries the instant ONE list read judges SLA state against.
type slaNowKey struct{}

// withPinnedSLANow fixes the SLA instant for everything built from ctx, so the
// page and the count beside it ask the same question of the same clock.
func withPinnedSLANow(ctx context.Context, now time.Time) context.Context {
	return context.WithValue(ctx, slaNowKey{}, now)
}

// slaNow is the pinned instant, or the live clock for a caller that pinned
// none — a single-statement read has nothing to agree with.
func slaNow(ctx context.Context) time.Time {
	if pinned, ok := ctx.Value(slaNowKey{}).(time.Time); ok {
		return pinned
	}
	return leadSLAClock().UTC()
}

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
	state := crmcontracts.LeadSlaStateLeadSlaStateWithinTarget
	switch {
	case now.After(deadline):
		state = crmcontracts.LeadSlaStateLeadSlaStateBreached
	case deadline.Sub(now) <= policy.atRisk():
		state = crmcontracts.LeadSlaStateLeadSlaStateAtRisk
	}
	return &deadline, &state
}

// A response target measures an actual incoming message, never a record
// arriving through creation or import.
var leadHasInboundSQL = auth.LiveLeadInquiryClause("lead")

var leadOwesAReplySQL = "archived_at IS NULL AND first_response_at IS NULL AND " + leadHasInboundSQL

// firstResponseAtSQL is the stamp a response at `at` earns on the lead under
// alias, firstResponseStamp in SQL: a predicate over many leads cannot call Go.
func firstResponseAtSQL(alias, at string) string {
	return "greatest(CAST(" + at + " AS timestamptz), " + alias + ".created_at)"
}

// leadTakesFirstResponseSQL is whether the live lead under alias takes a
// response at `at` as its first: no stamp holds it back that is not later. The
// subscriber's write and the day-answer sweep both ask it.
func leadTakesFirstResponseSQL(alias, at string) string {
	return fmt.Sprintf(`(%[1]s.archived_at IS NULL AND coalesce(%[1]s.first_response_at > %[2]s, true))`,
		alias, firstResponseAtSQL(alias, at))
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
func slaStateClause(ctx context.Context, policy leadSLAPolicy, state crmcontracts.ListLeadsParamsSlaState, arg func(any) int) string {
	if !policy.enabled {
		return "FALSE"
	}
	deadline := "COALESCE(routed_at, created_at) + $%d * interval '1 minute'"
	open := leadOwesAReplySQL + " AND "
	minutes := policy.targetMinutes()
	// The instant pinned on ctx, not a fresh reading: a list page and the total
	// beside it build this clause twice, and a lead crossing its deadline
	// between the two readings would be counted in one and not the other — a
	// total that disagrees with the page it labels, with no write to explain it.
	now := slaNow(ctx)
	switch crmcontracts.LeadSlaState(state) {
	case crmcontracts.LeadSlaStateLeadSlaStateBreached:
		return storekit.SQLf(open+deadline+" < $%d", arg(minutes), arg(now))
	case crmcontracts.LeadSlaStateLeadSlaStateAtRisk:
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
		var takes bool
		// A backdated activity can predate the lead it answers. The first
		// response is the earliest one, since the bus is at-least-once and
		// unordered; a later or equal stamp is a replay and a no-op.
		if err := tx.QueryRow(ctx, `SELECT first_response_at, `+firstResponseAtSQL("lead", "$2")+`, `+
			leadTakesFirstResponseSQL("lead", "$2")+` FROM lead WHERE id = $1`, leadID, at).Scan(&current, &at, &takes); err != nil {
			return err
		}
		if !takes {
			return nil
		}
		at = at.UTC()
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
	if t.direction != touchOutbound {
		return false
	}
	if humanCaptured(t) {
		return true
	}
	return t.hadInbound
}

// answeredBy is the ground truth the breach scan judges on. It is the earliest
// first response on this lead that answers by the deadline, or nil when there
// is none. A day touch answers once its whole date has ended.
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
		if earliest == nil || t.respondedAt.Before(*earliest) {
			at := t.respondedAt
			earliest = &at
		}
	}
	return earliest, nil
}

// leadTouchesFor answers this lead's activities that answer by the deadline
// (occurredday.ProvenSQL) in the shape isFirstResponseActivity reads.
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
		                 AND ai.occurred_at < a.occurred_at -- occurred_at raw: an inbound before the touch, an ordering of rows
		               ),
		       a.kind, coalesce(a.meeting_status, ''), a.source, `+occurredday.ProvenSQL("a")+`
		FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.lead_id = $1 AND a.archived_at IS NULL
		  AND `+occurredday.ProvenSQL("a")+` <= $2
		  AND `+auth.ActivityAvailableClause("a"), leadID, deadline)
	if err != nil {
		return nil, fmt.Errorf("read the lead's activities before its deadline: %w", err)
	}
	defer rows.Close()
	var out []leadResponseTouch
	for rows.Next() {
		t := leadResponseTouch{lead: leadID}
		if err := rows.Scan(&t.direction, &t.capturedBy, &t.occurredAt, &t.hadInbound,
			&t.kind, &t.meetingStatus, &t.source, &t.respondedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// IntakeSeat is the seat that answers for a breach nobody else can be given.
// It reads (id, true) when one is configured and can still work the queue.
// The scan and the delivery of its event share it.
func IntakeSeat(ctx context.Context, tx pgx.Tx) (ids.UUID, bool, error) {
	return unassignedEscalationTarget(ctx, tx, config.Bound())
}

// unassignedEscalationTarget reads the configured intake seat: (id, true) for a
// seat that can still work the queue, else (zero, false). A seat that is set
// but is no active colleague also logs a warning. The read is the ungated machinery
// one, since the SLA sweep runs as the system principal. A value that will not
// decode is an error: somebody wrote the row around its validator.
func unassignedEscalationTarget(ctx context.Context, tx pgx.Tx, cfg *config.Config) (ids.UUID, bool, error) {
	configured, err := config.StoredApplyTx(ctx, tx, cfg, UnassignedEscalationUserID)
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
			warnSeatAnswersNobody(ctx, cfg)
			return ids.UUID{}, false, nil
		}
		return ids.UUID{}, false, fmt.Errorf("check the unassigned escalation seat: %w", err)
	}
	return id, true, nil
}

// warnSeatAnswersNobody names the setting and where its value comes from. No
// save checks a boot value against the colleagues, so this is where an operator
// learns the seat they set answers for nothing.
func warnSeatAnswersNobody(ctx context.Context, cfg *config.Config) {
	source := "the database"
	if origin, locked := cfg.Locked(UnassignedEscalationUserID.Key()); locked {
		source = origin.String()
	}
	slog.WarnContext(ctx, "contacts: the unassigned escalation seat is not an active colleague, so a breach on a lead with no owner, or whose owner has left, escalates to nobody",
		"setting", UnassignedEscalationUserID.Key(), "origin", source)
}
