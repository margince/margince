// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deals a nightly input check examines, and what it could reach.
//
// assurance owns the questions and owns nothing about deals, offers or
// mailboxes, so the subjects arrive here rather than the module reaching for
// them. That keeps its rules pure — a rule is arithmetic over a struct, with no
// database to stand up — and keeps the reads where the authority lives.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closePushWindow bounds how far back the close-date-push count looks. One
// quarter: the closePushed rule asks whether the date keeps moving NOW, and a
// push from a year ago is history, not a pattern anybody should be paged about.
const closePushWindow = 90 * 24 * time.Hour

// AssuranceSubjects reads every live open deal the run should examine.
//
// Deliberately UNSCOPED, and that is a decision rather than an oversight. The
// pass runs as the system so it covers the whole pipeline: a duplicate-detection
// rule that saw only one rep's deals would report no duplicates, and a coverage
// count taken from a narrowed read would say the installation was checked when
// most of it was not.
//
// What that costs is paid on the way out, not here. An exception stores the
// subject's owner, and EVERY reader of the exception list passes through the
// caller's own row scope before rendering — the list endpoint, the headline
// counts, the brief candidate, the worklist row and the task subject line. A
// finding about a deal the reader cannot open is a finding they never see.
//
// Every field a rule reads is assembled here, and the two sides are held equal
// by TestEveryRuleInputHasAnAssembledSource: this seam once populated nine of
// fourteen fields, so a scheduled pass would have raised "no next step" and
// "no economic buyer" against every deal in the installation — the unpopulated
// zero value is exactly the shape those rules fire on.
//
// The two money comparisons only carry a total in the DEAL's own currency. An
// offer or contract in a different currency is not comparable minor-for-minor,
// and handing the rule such a number would report a "discrepancy" that is
// entirely exchange rate.
func AssuranceSubjects(ctx context.Context, tx pgx.Tx) ([]assurance.Subject, error) {
	now := time.Now().UTC()
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.owner_id, d.amount_minor, d.currency,
		       d.expected_close_date, d.close_date_provisional,
		       d.forecast_category, s.name,
		       (SELECT max(a.occurred_at)
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.deal_id = d.id
		           AND a.direction = 'inbound'
		           AND a.archived_at IS NULL) AS last_inbound_at,
		       (SELECT o.gross_minor
		          FROM offer o
		         WHERE o.deal_id = d.id
		           AND o.archived_at IS NULL
		           AND o.status IN ('sent', 'accepted')
		           AND o.currency = d.currency
		         ORDER BY o.updated_at DESC
		         LIMIT 1) AS offer_total_minor,
		       (SELECT c.value_minor
		          FROM contract c
		         WHERE c.deal_id = d.id
		           AND c.archived_at IS NULL
		           AND c.status = 'active'
		           AND c.currency = d.currency
		         ORDER BY c.signed_on DESC NULLS LAST, c.created_at DESC
		         LIMIT 1) AS contract_total_minor,
		       (SELECT count(*)
		          FROM audit_log au
		         WHERE au.entity_type = 'deal'
		           AND au.entity_id = d.id
		           AND au.action = 'update'
		           AND au.occurred_at >= $1
		           AND (au.before ->> 'expected_close_date') IS NOT NULL
		           AND (au.after ->> 'expected_close_date') IS NOT NULL
		           AND (au.after ->> 'expected_close_date')::date
		               > (au.before ->> 'expected_close_date')::date) AS close_date_pushes,
		       (SELECT a.subject
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.deal_id = d.id
		           AND a.archived_at IS NULL
		           AND ((a.kind = 'task' AND a.is_done = false)
		                OR (a.kind = 'meeting' AND a.meeting_status = 'booked'
		                    AND a.occurred_at > $2))
		         ORDER BY a.occurred_at
		         LIMIT 1) AS next_step,
		       EXISTS (SELECT 1
		          FROM relationship r
		         WHERE r.kind = 'deal_stakeholder'
		           AND r.deal_id = d.id
		           AND r.role = $3
		           AND r.archived_at IS NULL) AS has_economic_buyer
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE d.archived_at IS NULL AND d.status = 'open'
		ORDER BY d.id`,
		now.Add(-closePushWindow), now, dealrole.EconomicBuyer)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals to check: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.Subject, error) {
		var out assurance.Subject
		var dealID ids.UUID
		var owner *ids.UUID
		var currency, category, stage, nextStep *string
		var lastInbound *time.Time
		err := row.Scan(&dealID, &owner, &out.AmountMinor, &currency,
			&out.ExpectedClose, &out.CloseProvisional, &category, &stage, &lastInbound,
			&out.OfferTotalMinor, &out.ContractTotalMinor, &out.CloseDatePushes,
			&nextStep, &out.HasEconomicBuyer)
		out.DealID = dealID.String()
		if owner != nil {
			out.Owner = owner.String()
		}
		if currency != nil {
			out.Currency = *currency
		}
		if category != nil {
			out.Category = *category
		}
		if stage != nil {
			out.StageName = *stage
		}
		if nextStep != nil {
			out.NextStep = *nextStep
		}
		out.LastInboundAt = lastInbound
		return out, err
	})
}

// AssuranceCoverage answers which sources the run could reach.
//
// A source it cannot read is reported as unreachable rather than skipped: a run
// that silently omitted the mailbox would look identical to one that read it
// and found nothing, and the readiness rule reads absence as "we do not know"
// precisely so this cannot happen quietly.
func AssuranceCoverage(ctx context.Context, tx pgx.Tx, now time.Time) []assurance.SourceCoverage {
	return []assurance.SourceCoverage{
		mailCoverage(ctx, tx, now),
		offerCoverage(now),
	}
}

// mailProviders are the capture connectors that carry mail. gcal is calendar
// and the chat providers are messaging; a healthy WhatsApp says nothing about
// whether anybody read the mailbox a close date would be checked against.
var mailProviders = []string{"gmail", "imap", "graph", "offline_demo"}

// mailCoverage grades the mailbox by CONNECTOR health, not by mail volume.
//
// It used to read max(occurred_at) over captured email, which conflates two
// different facts: a quiet mailbox on a healthy connector was reported stale —
// nothing arrived, so "the source fell behind" — while a broken connector under
// recent mail read as checked. The sync checkpoint is the fact this state
// actually claims: when the connector last successfully read the source.
func mailCoverage(ctx context.Context, tx pgx.Tx, now time.Time) assurance.SourceCoverage {
	var connections int
	var reauth int
	var oldestSuccess, newestSuccess *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE c.status = 'reauth_required'),
		       min(s.last_success_at),
		       max(s.last_success_at)
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.provider = ANY($1) AND c.archived_at IS NULL`,
		mailProviders).Scan(&connections, &reauth, &oldestSuccess, &newestSuccess); err != nil {
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoverageUnavailable}
	}
	switch {
	case connections == 0:
		// Never configured. Different from broken: there is nothing to fix,
		// only something to decide, and the two route to different people.
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoverageNotConnected}
	case reauth > 0:
		// Somebody must re-grant access; until then part of the mailbox is
		// unreadable for a permission reason, not a technical one.
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoveragePermissionLimited}
	case oldestSuccess == nil:
		// Connected but no sync has ever succeeded.
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoverageUnavailable}
	case now.Sub(*oldestSuccess) > staleAfter:
		// The slowest mailbox is what the run's claim stands on: "mail was
		// checked" over three mailboxes means all three, so the oldest
		// checkpoint decides, and it also caps the checked-through date below.
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoverageStale}
	default:
		return assurance.SourceCoverage{
			Source: "mail", State: assurance.CoverageChecked, CheckedThrough: oldestSuccess,
		}
	}
}

// offerCoverage reports the native offers table.
//
// Always checked: the table lives in the same database as the run, so if this
// code executes at all the source was readable. It used to grade the table by
// its newest row's age, which reported "checked and found no offer" — an
// ordinary fact about most installations — as the source being unavailable.
func offerCoverage(now time.Time) assurance.SourceCoverage {
	return assurance.SourceCoverage{
		Source: "offers", State: assurance.CoverageChecked, CheckedThrough: &now,
	}
}

// staleAfter is how far behind a source may fall before a run should stop
// claiming to have checked it.
//
// Two days rather than one: a connector that ran yesterday evening and has not
// run today is normal, and a threshold that called that stale would report
// checks_incomplete every morning until somebody stopped reading the word.
const staleAfter = 48 * time.Hour
