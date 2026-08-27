// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TechnicalBackfillBatch bounds one sweep's nominations.
//
// Smaller than the geocode batch because each nomination is three outbound
// lookups rather than one, drained by a single worker against services that
// answer in seconds: fifty would be an hour of queue.
const TechnicalBackfillBatch = 25

// TechnicalRefreshAfter is how long a company's technical picture is trusted
// before the sweep asks again.
//
// Freshness IS the feature — the product's claim is that a mail system moving
// to Microsoft 365 shows up as a reason to call — so this is what decides
// whether that claim is true. A week is short enough that a move is noticed
// while it is still news and long enough that the public services this reads
// are not asked more often than an installation should ask them.
const TechnicalRefreshAfter = 7 * 24 * time.Hour

// ListTechnicalDue names the companies whose technical picture should be read.
//
// Three kinds of company are due, and the third is why this exists as a
// scheduled pass rather than only as a trigger:
//
//   - never asked — recorded before the deployment could look, or before the
//     record carried a domain;
//   - asked and failed, past the backoff its ledger recorded;
//   - asked, succeeded, and gone STALE. Nothing writes a company row when its
//     mail provider changes at the company, so no trigger can fire. Only
//     coming back round observes it.
//
// A company with no domain is never due: the lookup reads what the record
// holds, and there is nothing to ask about.
func (s *Store) ListTechnicalDue(ctx context.Context, limit int, now time.Time) ([]ids.OrganizationID, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = TechnicalBackfillBatch
	}
	var due []ids.OrganizationID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// A company is due when ANY lane is: either a lane has never run (no
		// row for it at all) or the row it has is past its own next_attempt_at.
		// Counting the settled lanes and comparing against the lane count is
		// how "any" is asked without a per-lane join — a company all of whose
		// lanes are settled and in date has as many settled rows as there are
		// lanes, and every other company has fewer.
		rows, err := tx.Query(ctx, `
			SELECT o.id
			  FROM organization o
			 WHERE o.archived_at IS NULL
			   AND NOT o.is_anchor
			   AND EXISTS (SELECT 1 FROM organization_domain d WHERE d.organization_id = o.id)
			   AND (SELECT count(*) FROM organization_technical_state s
			         WHERE s.organization_id = o.id
			           AND s.next_attempt_at IS NOT NULL
			           AND s.next_attempt_at > $2) < $3
			 ORDER BY o.id
			 LIMIT $1`,
			limit, now, len(technicalLanes))
		if err != nil {
			return fmt.Errorf("list companies due a technical lookup: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var orgID ids.OrganizationID
			if err := rows.Scan(&orgID); err != nil {
				return fmt.Errorf("list companies due a technical lookup: %w", err)
			}
			due = append(due, orgID)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return due, nil
}

// TechnicalDomain reads the domain a technical lookup may ask about.
//
// THE ONLY SOURCE OF THE DOMAIN. Nothing in this lane accepts one from a
// request body or a job argument, so a caller cannot point the lookup at a
// company the workspace has not recorded — which is the guardrail that keeps
// this path from becoming company discovery.
//
// Reports false when the record carries none, which is a refusal rather than a
// failure: there is simply nothing to look up.
//
// Held by: TestTheTechnicalLookupTakesNoDomainFromACaller (backend/gates/technicaldomain_test.go),
// with TestTheTechnicalLookupReadsTheDomainFromTheRecordAlone holding the read side.
func (s *Store) TechnicalDomain(ctx context.Context, orgID ids.OrganizationID) (string, bool, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return "", false, err
	}
	var domain string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "organization", orgID.UUID); err != nil {
			return err
		}
		// The PRIMARY domain when the record names one, else the oldest — the
		// same order the record itself displays, so the lookup reads what a
		// person looking at the page would call the company's domain.
		return tx.QueryRow(ctx, `
			SELECT domain
			  FROM organization_domain
			 WHERE organization_id = $1
			 ORDER BY is_primary DESC, created_at ASC
			 LIMIT 1`, orgID).Scan(&domain)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read the company's domain: %w", err)
	}
	return domain, domain != "", nil
}

// RecordTechnicalLane writes what one lane did, and when to ask again.
//
// Per lane rather than per run because the three sources fail independently: a
// certificate log that has been down for a week must not make a mail provider
// read this morning look stale, and it must not have its own backoff reset by
// the DNS lane succeeding.
func (s *Store) RecordTechnicalLane(
	ctx context.Context, orgID ids.OrganizationID, lane TechnicalLane, outcome string, now time.Time,
) error {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return err
	}
	succeeded := outcome == TechnicalOutcomeApplied || outcome == TechnicalOutcomeEmpty ||
		outcome == TechnicalOutcomeRefused
	return s.tx(ctx, func(tx pgx.Tx) error {
		// Row-scoped like every other write: object RBAC says this caller may
		// update organizations, and this says WHICH. Without it an admitted
		// caller could write a ledger row for an organization outside its
		// scope — and the ledger is what decides when that company is read.
		if err := auth.EnsureWritableLive(ctx, tx, "organization", orgID.UUID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO organization_technical_state
			  (organization_id, lane, attempts, last_outcome, last_success_at, next_attempt_at, updated_at)
			VALUES ($1, $2, CASE WHEN $4 THEN 0 ELSE 1 END, $3,
			        CASE WHEN $4 THEN $5::timestamptz END, $5::timestamptz + $6::interval, now())
			ON CONFLICT (organization_id, lane)
			DO UPDATE SET
			  -- Reset on success, climb on failure: the attempt count is what
			  -- the backoff is computed from, so a lane that recovers must not
			  -- keep the delay it earned while broken.
			  attempts = CASE WHEN $4 THEN 0 ELSE organization_technical_state.attempts + 1 END,
			  last_outcome = EXCLUDED.last_outcome,
			  last_success_at = CASE WHEN $4 THEN EXCLUDED.last_success_at
			                         ELSE organization_technical_state.last_success_at END,
			  next_attempt_at = EXCLUDED.next_attempt_at,
			  updated_at = now()`,
			orgID, string(lane), outcome, succeeded, now, technicalBackoff(outcome).String())
		if err != nil {
			return fmt.Errorf("record what the %s lane did: %w", lane, err)
		}
		return nil
	})
}

// The outcomes a lane can report, matching the contract's own vocabulary.
const (
	// TechnicalOutcomeApplied is a lane that answered and had something.
	TechnicalOutcomeApplied = "applied"
	// TechnicalOutcomeEmpty is a lane that answered and the company publishes
	// none of what it reads. An ANSWER, and one worth recording: it is what
	// stops the sweep asking again tomorrow.
	TechnicalOutcomeEmpty = "empty"
	// TechnicalOutcomeFailed is a lookup that did not complete. It changes
	// nothing on the record.
	TechnicalOutcomeFailed = "failed"
	// TechnicalOutcomeRefused is the site's robots.txt declining. A settled
	// answer rather than a failure — asking again next week is polite, asking
	// again tomorrow is not.
	TechnicalOutcomeRefused = "refused"
)

// technicalBackoff is how long before this lane is asked again.
func technicalBackoff(outcome string) time.Duration {
	if outcome == TechnicalOutcomeFailed {
		// Short, because a failure is usually the far end having a bad hour
		// rather than a settled state — crt.sh in particular.
		return 6 * time.Hour
	}
	return TechnicalRefreshAfter
}

// TechnicalLaneState is what one public source last did for one company.
type TechnicalLaneState struct {
	Lane          string
	Attempts      int
	Outcome       string
	LastSuccessAt *time.Time
	NextAttemptAt *time.Time
}

// TechnicalLaneState reads the ledger for one company, a row per lane that has
// ever run.
//
// An empty result is "never looked up", which the caller reports as a 404 —
// the honest difference between a company nobody has asked about and one whose
// sources answered and had nothing.
func (s *Store) TechnicalLaneState(ctx context.Context, orgID ids.OrganizationID) ([]TechnicalLaneState, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return nil, err
	}
	var lanes []TechnicalLaneState
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Row-scoped like any other read: a leaked org id buys nothing, and the
		// miss is an existence-hiding 404 rather than an empty ledger.
		if err := auth.EnsureVisible(ctx, tx, "organization", orgID.UUID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT lane, attempts, coalesce(last_outcome, ''), last_success_at, next_attempt_at
			  FROM organization_technical_state
			 WHERE organization_id = $1
			 ORDER BY lane`, orgID)
		if err != nil {
			return fmt.Errorf("read what the technical lookup last did: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var lane TechnicalLaneState
			if err := rows.Scan(&lane.Lane, &lane.Attempts, &lane.Outcome,
				&lane.LastSuccessAt, &lane.NextAttemptAt); err != nil {
				return fmt.Errorf("read what the technical lookup last did: %w", err)
			}
			lanes = append(lanes, lane)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return lanes, nil
}
