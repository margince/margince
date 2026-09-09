// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The captured-company auto-enrich sweep's store (CAP-PARAM-7,
// ADR-0072/A118): the per-company attempt cursor (capture_auto_enrich_state), the
// installation-wide daily spend cap (capture_auto_enrich_budget), and the due-company
// candidate read. Compose owns the sweep worker and the deep-read enqueue; this
// store owns the scheduling state and the atomic cap reservation so the two are
// one transaction each. The candidate read joins company / site_read
// (people-owned) — a read is bounded by its own workspace predicate, not by
// which module owns the table — so all the sweep's eligibility logic lives in
// one place.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// autoEnrichMaxAttempts bounds how many times the sweep re-enqueues a deep-read
// for one company before giving up (ADR-0072: retries=2). A read that
// applied or evidenced nothing is terminal (next_attempt_at NULL) and never
// counts against this; only a failed read consumes an attempt.
const autoEnrichMaxAttempts = 2

// DueCompany is one company the sweep should enrich: its id and the primary
// domain that seeds the crawl.
type DueCompany struct {
	CompanyID ids.CompanyID
	Domain         string
}

// AutoEnrichStore owns the sweep's scheduling state and daily-cap reservation.
type AutoEnrichStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
}

// NewAutoEnrichStore builds the store on a handle already bound to the
// workspace it serves.
func NewAutoEnrichStore(db *database.DB) *AutoEnrichStore { return &AutoEnrichStore{db: db} }

// ListDueCompanies returns up to limit captured companies that need a dossier,
// OLDEST first: with a live primary domain, no dossier, and either no cursor
// row or a due one under the attempt bound. The query's own workspace predicate
// scopes it to the bound workspace.
//
// The population is every company with a live primary domain and no dossier,
// however it was named. ADR-0072 scoped this lane to auto-created companies
// (name_source='domain') — "the company nobody has named". Lars overruled that
// on 2026-08-19, twice: "when I create a company and if admin enabled automatic
// website read then this should be also put on the list for website ingestion",
// and "If I manually create a company auto enrich must also be triggered." A
// person typing the name is not a reason to withhold the dossier; it is usually
// the moment they want one.
//
// The old rule had also made the lane inert rather than selective: all 195
// companies in the demo workspace carry name_source='human', so the sweep
// had no candidate at all and had never enriched anything.
//
// The anchor stays out, now by its OWN predicate rather than as a side effect
// of being human-named — removing name_source without this would have started
// offering it. Two reasons it does not belong here. It is enriched during cold
// start already (people/company.go's cold-start read-back, distinct from the
// human write), so a sweep over it is redundant — Lars, 2026-08-20: "Gradion
// (anchor) IS enriched during coldstart and does not require auto enrichment
// later." And this lane applies what it finds directly, while the anchor's own
// refresh compares every proposal against confirmed truth and asks the human to
// resolve each conflict (ADR-0065 §8); offering it here would write machine
// values onto the one company the installation IS, outside that comparison.
//
// Oldest first, and that ordering is the whole termination argument.
//
// It used to take the NEWEST prefix, which starves. A pass queues what it takes
// — writing a cursor row that drops the company out of this set until its next
// attempt is due — so the set shrinks by exactly what was worked. Taking the
// oldest end therefore drains it in arrival order: every company ahead of a
// given one is consumed before it, and nothing new can be inserted ahead of it
// because nothing new is older. Taking the NEWEST end reverses that: in a
// workspace gaining more eligible companies between passes than the daily cap,
// arrivals keep landing in front of a company that has never been reached, and
// it waits forever — not retried, not exhausted, not visible as skipped.
//
// The order is on the ID rather than created_at because it must be TOTAL: two
// companies captured in the same statement share an instant, and a tie under a
// LIMIT is a coin toss that can seat the same row at the boundary every pass.
//
// TOTAL and STABLE is all the argument above needs. A uuidv7 sorts by its
// millisecond and then by bits that are process-local or random, so ids minted
// on two app instances inside one millisecond can interleave — it is very
// nearly arrival order, not exactly it. That costs nothing here: a company can
// be overtaken only by rows sharing its millisecond, which is a bounded
// reordering and not a queue it can sit behind. What would break the argument
// is an order that is not total, or one a new row can enter arbitrarily far
// ahead in — and the id is neither.
//
// The company-name sweep pages this same set to exhaustion instead
// (people/companynamepromotion.go). It can: its per-candidate work is one in-memory
// decision. This lane's is a model-backed site read under a daily cap, so a
// pass takes one page by construction — which is why the ordering has to be the
// thing that guarantees progress.
//
// The site-read clause excludes a company whose dossier exists or is still
// coming. A read that ended with NO dossier is not one: a failure, and a
// cancellation — the operator had auto-enrich off when the worker claimed the
// read. Reading a cancellation as a dossier would make turning the setting back
// on a permanent no-op for every company whose read it stopped, which is the
// opposite of the self-healing this sweep is for.
func (s *AutoEnrichStore) ListDueCompanies(ctx context.Context, limit int) ([]DueCompany, error) {
	var out []DueCompany
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, d.domain
			FROM company o
			JOIN company_domain d
			  ON d.company_id = o.id AND d.is_primary AND d.archived_at IS NULL
			LEFT JOIN capture_auto_enrich_state s ON s.company_id = o.id
			WHERE o.archived_at IS NULL
			  AND NOT o.is_anchor
			  AND NOT EXISTS (
				SELECT 1 FROM site_read sr
				WHERE sr.company_id = o.id
				  AND sr.status NOT IN ('failed', 'cancelled'))
			  AND (
				s.company_id IS NULL
				OR (s.next_attempt_at IS NOT NULL AND s.next_attempt_at <= now()
				    AND s.attempts < $1))
			ORDER BY o.id
			LIMIT $2`, autoEnrichMaxAttempts, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o DueCompany
			if err := rows.Scan(&o.CompanyID, &o.Domain); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing companies due for auto-enrich: %w", err)
	}
	return out, nil
}

// ExpireExhausted retires the cursors of companies that have used every attempt
// without a dossier landing: it sets last_outcome='exhausted' and clears
// next_attempt_at, so the row drops out of the partial due-index (it is no
// longer re-scanned every pass) — the real termination the 'exhausted' state
// names. Called once per sweep pass, before ListDueCompanies. A resolved company already
// has a NULL next_attempt_at, so the NOT-NULL guard leaves it untouched.
func (s *AutoEnrichStore) ExpireExhausted(ctx context.Context) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_auto_enrich_state SET
			  last_outcome = 'exhausted', next_attempt_at = NULL, updated_at = now()
			WHERE attempts >= $1 AND next_attempt_at IS NOT NULL`, autoEnrichMaxAttempts)
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: expiring exhausted auto-enrich cursors: %w", err)
	}
	return nil
}

// BudgetSlot is one reservation against the installation's daily read allowance: the
// UTC day it was taken on, and whether it was granted at all. Carry it from the
// reservation to the refund — the day is what makes a refund land on the row the
// reservation incremented.
type BudgetSlot struct {
	Day      time.Time
	Reserved bool
}

// ReserveBudget atomically reserves one auto-enrich slot for the current UTC
// day, returning false when the daily cap is already spent. The counter is
// keyed on the date alone, so every workspace pass spends from the one pot. The
// reservation is the same transaction as the counter read, so two concurrent
// sweeps (replicas) can never both slip past the cap.
func (s *AutoEnrichStore) ReserveBudget(ctx context.Context, dailyCap int) (BudgetSlot, error) {
	var slot BudgetSlot
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var enqueued int
		var day time.Time
		// INSERT the day's first slot, or increment only while under the cap;
		// the WHERE on DO UPDATE makes an over-cap increment a no-op that
		// RETURNS nothing, so the reservation is atomic.
		err := tx.QueryRow(ctx, `
			INSERT INTO capture_auto_enrich_budget (budget_date, enqueued)
			VALUES ((now() AT TIME ZONE 'UTC')::date, 1)
			ON CONFLICT (budget_date)
			DO UPDATE SET enqueued = capture_auto_enrich_budget.enqueued + 1
			WHERE capture_auto_enrich_budget.enqueued < $1
			RETURNING enqueued, budget_date`, dailyCap).Scan(&enqueued, &day)
		if err == nil {
			slot = BudgetSlot{Day: day, Reserved: true}
			return nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			// The DO UPDATE WHERE failed the cap guard: nothing reserved. (A
			// real error would fall through and abort the workspace pass.)
			return nil
		}
		return err
	})
	if err != nil {
		return BudgetSlot{}, fmt.Errorf("capture: reserving auto-enrich budget: %w", err)
	}
	return slot, nil
}

// ReleaseBudget returns one reserved slot to the day, for a reservation that
// bought nothing.
//
// The pattern is reserve-before-spend, which means the caller sometimes holds a
// slot it turns out not to need: two paths racing on one company both
// reserve, and the uniqueness index lets only one of them start a read. Without
// the refund the day's allowance erodes a slot at a time, and the shortfall grows
// with exactly the concurrency the cap is meant to be indifferent to. A slot that
// was never granted refunds nothing.
//
// Guarded at zero rather than trusted: a decrement that could run below zero
// would hand out free reads on the next reservation, which is the failure this
// counter exists to prevent.
//
// It refunds the day the slot was RESERVED on, not today. A read that started
// at 23:59:59 and joined after midnight would otherwise decrement the new day's
// row — freeing a slot nobody had taken and letting that day start one read past
// its cap. The counter is per UTC day, so the refund has to name the same day
// the reservation did.
func (s *AutoEnrichStore) ReleaseBudget(ctx context.Context, slot BudgetSlot) error {
	if !slot.Reserved {
		return nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_auto_enrich_budget
			   SET enqueued = enqueued - 1
			 WHERE budget_date = $1
			   AND enqueued > 0`, slot.Day)
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: releasing an auto-enrich budget slot: %w", err)
	}
	return nil
}

// MarkQueued records that the sweep enqueued a deep-read for companyID: it counts
// the attempt and arms next_attempt_at at the failure backoff, so a job that
// never completes is re-driven after the backoff, up to the attempt bound. A
// terminal outcome (MarkResolved) clears next_attempt_at.
// The backoff is applied to the DATABASE's clock, because the due-scan compares
// next_attempt_at against Postgres now(). Deriving the deadline from the app
// process instead makes that a cross-clock comparison, and the two clocks are
// only ever coincidentally equal.
func (s *AutoEnrichStore) MarkQueued(ctx context.Context, companyID ids.CompanyID, backoff time.Duration) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_auto_enrich_state
			  (company_id, attempts, last_attempt_at, next_attempt_at, last_outcome)
			VALUES ($1, 1, now(), now() + make_interval(secs => $2), 'queued')
			ON CONFLICT (company_id) DO UPDATE SET
			  attempts = capture_auto_enrich_state.attempts + 1,
			  last_attempt_at = now(),
			  next_attempt_at = now() + make_interval(secs => $2),
			  last_outcome = 'queued',
			  updated_at = now()`, companyID, backoff.Seconds())
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: marking auto-enrich queued: %w", err)
	}
	return nil
}

// MarkResolved records the terminal outcome of a deep-read the sweep triggered.
// 'applied' and 'empty' are terminal — next_attempt_at is cleared so the company is
// never re-enqueued; 'failed' leaves the queued backoff standing so the next
// due sweep retries it (until the attempt bound). A cursor row is expected
// (MarkQueued wrote it); a missing row is a no-op, never an error.
func (s *AutoEnrichStore) MarkResolved(ctx context.Context, companyID ids.CompanyID, outcome string) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_auto_enrich_state SET
			  last_outcome = $2,
			  next_attempt_at = CASE WHEN $2 IN ('applied', 'empty') THEN NULL ELSE next_attempt_at END,
			  updated_at = now()
			WHERE company_id = $1`, companyID, outcome)
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: marking auto-enrich resolved: %w", err)
	}
	return nil
}
