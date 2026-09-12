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
			WHERE `+leadOwesAReplySQL+` AND sla_breached_at IS NULL
			  -- Only a lead somebody ASKED US FOR. A name read off a public web
			  -- page is a record that the contact exists: nobody wrote in, no
			  -- first response is owed, and escalating one would stamp
			  -- sla_breached_at against a promise that was never made.
			  --
			  -- Not narrowed by OWNERSHIP, which is a different question: an
			  -- unowned inbound lead is precisely what the configured intake
			  -- seat answers for, and dropping those rows retires that seat.
			  --
			  -- A NULL source system is scanned. A direct create sets none, and
			  -- a lead somebody typed in by hand is the clearest case of work
			  -- owed — the same way round as the kernel's payload test.
			  AND (source_system IS NULL OR source_system <> ALL($3))
			  AND COALESCE(routed_at, created_at) + $1 * interval '1 minute' < $2
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED`,
			policy.targetMinutes(), now, leadsource.PassiveDiscoverySources())
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
	switch {
	case b.OwnerID != nil:
		owner := openapi_types.UUID(b.OwnerID.UUID)
		payload.OwnerId = &owner
		// An owned lead escalates to its OWNER: the desk that owes the answer
		// is the desk that hears about the miss.
		payload.EscalationTarget = &owner
	default:
		// A lead NOBODY owns is the queue's worst case — past its response
		// target with no one who has picked it up — and it was the one case
		// that reached nobody at all: an unassigned task and no notice.
		//
		// The configured intake seat answers for it. Unset means the
		// installation has not said who runs the queue, and the breach stays
		// where it was rather than being addressed to somebody who did not
		// agree to it; the unassigned view is what surfaces those.
		target, configured, err := unassignedEscalationTarget(ctx, tx)
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
