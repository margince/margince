// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The at-most-once first-response breach sweep: which leads it admits, and
// what marking one does.
//
// Its own file because it is one concept with its own rules — who is owed a
// reply, who is answerable for it, and the single row that records the miss —
// and because leadsla.go had grown past the length a reader can hold at once.
// The clock those rules are read against, and what counts as a response, stay
// next door: this file decides who to escalate to, not when a promise is late.

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/leadsource"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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
// With the target switched off the scan marks no breach. No deadline exists to
// breach, and a breach row nobody asked for would page an owner. It still stamps
// day answers.
func (s *Store) ScanLeadSLA(ctx context.Context, now time.Time) ([]SLABreach, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return nil, err
	}
	var breaches []SLABreach
	err := s.tx(ctx, func(tx pgx.Tx) error {
		policy, err := loadLeadSLAPolicy(ctx, tx, s.bootConfig())
		if err != nil {
			return err
		}
		if policy.enabled {
			if breaches, err = markBreaches(ctx, tx, policy, now); err != nil {
				return err
			}
		}
		// After the breaches: a date that ended past the deadline answered late,
		// and stamped first the lead would no longer owe the reply it missed.
		return stampDayAnswers(ctx, tx, now)
	})
	return breaches, err
}

// markBreaches marks each lead whose deadline under policy passed unanswered.
func markBreaches(ctx context.Context, tx pgx.Tx, policy leadSLAPolicy, now time.Time) ([]SLABreach, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, owner_id, COALESCE(routed_at, created_at) + $1 * interval '1 minute',
		       COALESCE(NULLIF(btrim(full_name), ''), email::text, '')
		FROM lead
		WHERE `+leadOwesAReplySQL+` AND sla_breached_at IS NULL
		  AND (source_system IS NULL OR source_system <> ALL($3))
		  AND COALESCE(routed_at, created_at) + $1 * interval '1 minute' < $2
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED`,
		policy.targetMinutes(), now, leadsource.PassiveDiscoverySources())
	if err != nil {
		return nil, fmt.Errorf("select breached leads: %w", err)
	}
	candidates, err := collectBreaches(rows)
	if err != nil {
		return nil, err
	}
	// first_response_at is a projection the bus writes late, and a breach pages
	// someone. So each candidate is judged on its activities, and an answer found
	// there is stamped here; the subscriber's later write is then a replay.
	var breaches []SLABreach
	for _, b := range candidates {
		answered, err := answeredBy(ctx, tx, b.LeadID, b.Deadline)
		if err != nil {
			return nil, err
		}
		if answered != nil {
			if _, err := recordFirstResponseTx(ctx, tx, b.LeadID, *answered); err != nil {
				return nil, err
			}
			continue
		}
		if err := markBreach(ctx, tx, b, now); err != nil {
			return nil, err
		}
		breaches = append(breaches, b)
	}
	return breaches, nil
}

// stampDayAnswers stamps the first response of each lead that a day touch has
// answered by now. The subscriber skips a day touch while its date runs, and
// this scan runs on the clock whether or not a target is set. It reads the
// candidates a page at a time, so no scan holds them all at once.
func stampDayAnswers(ctx context.Context, tx pgx.Tx, now time.Time) error {
	var after ids.LeadID
	for {
		leads, err := dayAnswerCandidates(ctx, tx, now, after)
		if err != nil {
			return err
		}
		for _, lead := range leads {
			answered, err := answeredBy(ctx, tx, lead, now)
			if err != nil {
				return err
			}
			if answered == nil {
				continue
			}
			if _, err := recordFirstResponseTx(ctx, tx, lead, *answered); err != nil {
				return err
			}
		}
		if len(leads) < dayAnswerPage {
			return nil
		}
		after = leads[len(leads)-1]
	}
}

// dayAnswerPage is how many leads one read of the day-answer sweep returns.
const dayAnswerPage = 500

// dayAnswerCandidates are the leads after `after`, in id order, holding a day
// touch that may answer them by now: one mayAnswerSQL admits, whose answer the
// lead takes (leadTakesFirstResponseSQL), as the subscriber's write asks.
func dayAnswerCandidates(ctx context.Context, tx pgx.Tx, now time.Time, after ids.LeadID) ([]ids.LeadID, error) {
	proven := occurredday.ProvenSQL("a")
	rows, err := tx.Query(ctx, `
		SELECT lead.id FROM lead
		 WHERE lead.id > $2
		   AND EXISTS (SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
		                WHERE l.lead_id = lead.id AND a.archived_at IS NULL AND a.occurred_precision = 'day'
		                  AND `+mayAnswerSQL("a")+` AND `+proven+` <= $1
		                  AND `+leadTakesFirstResponseSQL("lead", proven)+`
		                  AND `+auth.ActivityAvailableClause("a")+`)
		 ORDER BY lead.id LIMIT $3`, now, after, dayAnswerPage)
	if err != nil {
		return nil, fmt.Errorf("select leads a day touch may have answered: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.LeadID])
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

// SeatAnswersBreach says whether a seat can be handed a lead's escalation: an
// active human colleague whose seat can write. It asks auth.EnsureAssignee,
// which also vets the intake seat. The scan and the delivery of its event share it.
func SeatAnswersBreach(ctx context.Context, tx pgx.Tx, seat ids.UUID) (bool, error) {
	err := auth.EnsureAssignee(ctx, tx, seat)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return false, nil
	}
	return err == nil, err
}

func (b SLABreach) ownerAnswers(ctx context.Context, tx pgx.Tx) (bool, error) {
	if b.OwnerID == nil {
		return false, nil
	}
	return SeatAnswersBreach(ctx, tx, b.OwnerID.UUID)
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
	ownerAnswers, err := b.ownerAnswers(ctx, tx)
	if err != nil {
		return err
	}
	if b.OwnerID != nil {
		owner := openapi_types.UUID(b.OwnerID.UUID)
		payload.OwnerId = &owner
	}
	switch {
	case ownerAnswers:
		// An owned lead escalates to its OWNER: the desk that owes the answer
		// is the desk that hears about the miss.
		payload.EscalationTarget = payload.OwnerId
	default:
		// A lead nobody answers for is the queue's worst case. An owner who has
		// left is that case too: their task would be refused and their notice
		// read by nobody. The configured intake seat answers for it.
		//
		// Unset means nobody has said who runs the queue, so the breach stays
		// unaddressed rather than going to someone who never agreed to it.
		// The unassigned view surfaces those.
		target, configured, err := IntakeSeat(ctx, tx)
		if err != nil {
			return err
		}
		if configured {
			addressed := openapi_types.UUID(target)
			payload.EscalationTarget = &addressed
		}
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, b.LeadID.UUID, payload); err != nil {
		return fmt.Errorf("emit lead.sla_breached: %w", err)
	}
	return nil
}
