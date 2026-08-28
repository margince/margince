// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The lead first-response SLA (formulas §18): the deterministic clock, what
// counts as a first response, and the at-most-once breach scan. The target
// is the §18 default; the RC-5 per-workspace override is not wired here yet.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
func slaStateClause(policy leadSLAPolicy, state crmcontracts.ListLeadsParamsSlaState, arg func(any) int) string {
	if !policy.enabled {
		return "FALSE"
	}
	deadline := "COALESCE(routed_at, created_at) + $%d * interval '1 minute'"
	open := "archived_at IS NULL AND first_response_at IS NULL AND "
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

// ScanLeadSLA marks every open lead whose first-response deadline has passed
// unanswered and not yet been escalated (formulas §18.2), once per breach:
// sla_breached_at is the at-most-once mark, and the row lock with SKIP
// LOCKED lets two scans share the work without escalating a lead twice.
// Each breach lands one audit row and lead.sla_breached; the escalation
// task hangs off that event.
//
// With the target switched off the scan is a no-op: no deadline exists to
// breach, and a breach row the installation never asked for would page an
// owner about a rule they did not set.
func (s *Store) ScanLeadSLA(ctx context.Context, now time.Time) ([]SLABreach, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return nil, err
	}
	var breaches []SLABreach
	err := s.tx(ctx, func(tx pgx.Tx) error {
		policy, err := loadLeadSLAPolicy(ctx, tx)
		if err != nil {
			return err
		}
		if !policy.enabled {
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT id, owner_id, COALESCE(routed_at, created_at) + $1 * interval '1 minute',
			       COALESCE(NULLIF(btrim(full_name), ''), email::text, '')
			FROM lead
			WHERE archived_at IS NULL AND first_response_at IS NULL AND sla_breached_at IS NULL
			  AND COALESCE(routed_at, created_at) + $1 * interval '1 minute' < $2
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED`,
			policy.targetMinutes(), now)
		if err != nil {
			return fmt.Errorf("select breached leads: %w", err)
		}
		candidates, err := collectBreaches(rows)
		if err != nil {
			return err
		}
		// Each candidate is re-checked against the ACTIVITIES before it is
		// marked. first_response_at is projected by an event subscriber
		// reacting to activity.captured, so it lags the activity's own commit
		// by the bus's latency: a reply committed at 11:59 against a 12:00
		// deadline can still be unprojected when a 12:01 scan runs, and the
		// lead reads as unanswered because the projection has not caught up
		// rather than because nobody answered.
		//
		// The scan owns the consequence — a breach event, an escalation task,
		// and a number on somebody's review — so it reads the ground truth
		// rather than the projection. Where it finds one, it stamps the column
		// itself: the subscriber's later write is then the replay
		// recordFirstResponseTx already answers as a no-op.
		breaches = breaches[:0]
		for _, b := range candidates {
			answered, err := answeredBy(ctx, tx, b.LeadID, b.Deadline)
			if err != nil {
				return err
			}
			if answered != nil {
				if _, err := recordFirstResponseTx(ctx, tx, b.LeadID, *answered); err != nil {
					return err
				}
				continue
			}
			if err := markBreach(ctx, tx, b, now); err != nil {
				return err
			}
			breaches = append(breaches, b)
		}
		return nil
	})
	return breaches, err
}

func collectBreaches(rows pgx.Rows) ([]SLABreach, error) {
	defer rows.Close()
	var out []SLABreach
	for rows.Next() {
		var b SLABreach
		if err := rows.Scan(&b.LeadID, &b.OwnerID, &b.Deadline, &b.Name); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func markBreach(ctx context.Context, tx pgx.Tx, b SLABreach, now time.Time) error {
	// The SLA sweep runs as an unbounded system principal, for whom this probe
	// returns nil without a query. It is here so that the day a human-facing
	// caller reaches this write — an operator forcing a breach, a retry surface —
	// it arrives already scoped rather than silently unguarded.
	if err := auth.EnsureWritableLive(ctx, tx, "lead", b.LeadID.UUID); err != nil {
		return err
	}
	// The row is already locked by the scan's SELECT ... FOR UPDATE; the
	// predicate is the CAS that keeps the mark at-most-once regardless.
	tag, err := tx.Exec(ctx,
		`UPDATE lead SET sla_breached_at = $2 WHERE id = $1 AND sla_breached_at IS NULL`, b.LeadID, now)
	if err != nil {
		return fmt.Errorf("mark sla breach: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark sla breach on %s: %w", b.LeadID, apperrors.ErrConflict)
	}
	// sla_breached_at is the lead's own column and it is the whole change; the
	// deadline that was missed is context ABOUT the breach and rides evidence,
	// because a lead has no `deadline` column and field history would project
	// one as a field of the record that nobody can find.
	//
	// The before-image is exact rather than read: the UPDATE above is the
	// at-most-once CAS, so the single row it affected held nothing here.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "lead", b.LeadID.UUID,
		map[string]any{slaBreachedColumn: nil}, map[string]any{slaBreachedColumn: now},
		map[string]any{"deadline": b.Deadline})
	if err != nil {
		return fmt.Errorf("audit sla breach: %w", err)
	}
	payload := crmcontracts.PublicEventLeadSlaBreached{Deadline: b.Deadline}
	if b.OwnerID != nil {
		owner := openapi_types.UUID(b.OwnerID.UUID)
		payload.OwnerId = &owner
		// Until a team-lead concept exists to resolve the §18 escalation
		// target through, the owner IS the target: the breach lands on the
		// desk that owns the lead rather than nowhere.
		payload.EscalationTarget = &owner
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, b.LeadID.UUID, payload); err != nil {
		return fmt.Errorf("emit lead.sla_breached: %w", err)
	}
	return nil
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
