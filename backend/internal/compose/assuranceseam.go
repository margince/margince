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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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
func AssuranceSubjects(ctx context.Context, tx pgx.Tx) ([]assurance.Subject, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.owner_id, d.amount_minor, d.currency,
		       d.expected_close_date, d.close_date_provisional,
		       d.forecast_category, s.name,
		       (SELECT max(a.occurred_at)
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.entity_type = 'deal' AND al.entity_id = d.id
		           AND a.direction = 'inbound'
		           AND a.archived_at IS NULL) AS last_inbound_at
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE d.archived_at IS NULL AND d.status = 'open'
		ORDER BY d.id`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals to check: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.Subject, error) {
		var out assurance.Subject
		var dealID ids.UUID
		var owner *ids.UUID
		var currency, category, stage *string
		var lastInbound *time.Time
		err := row.Scan(&dealID, &owner, &out.AmountMinor, &currency,
			&out.ExpectedClose, &out.CloseProvisional, &category, &stage, &lastInbound)
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
		offerCoverage(ctx, tx, now),
	}
}

// mailCoverage says whether the installation's mail is current enough to check
// a close date against what a buyer actually said.
func mailCoverage(ctx context.Context, tx pgx.Tx, now time.Time) assurance.SourceCoverage {
	var newest *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT max(occurred_at) FROM activity
		  WHERE kind = 'email' AND archived_at IS NULL`).Scan(&newest); err != nil {
		// The read itself failed. Unavailable, not stale: we did not get far
		// enough to say how current anything is.
		return assurance.SourceCoverage{Source: "mail", State: assurance.CoverageUnavailable}
	}
	return freshness("mail", newest, now)
}

// offerCoverage says whether the offers an amount would be checked against are
// current.
func offerCoverage(ctx context.Context, tx pgx.Tx, now time.Time) assurance.SourceCoverage {
	var newest *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT max(updated_at) FROM offer WHERE archived_at IS NULL`).Scan(&newest); err != nil {
		return assurance.SourceCoverage{Source: "offers", State: assurance.CoverageUnavailable}
	}
	return freshness("offers", newest, now)
}

// staleAfter is how far behind a source may fall before a run should stop
// claiming to have checked it.
//
// Two days rather than one: a connector that ran yesterday evening and has not
// run today is normal, and a threshold that called that stale would report
// checks_incomplete every morning until somebody stopped reading the word.
const staleAfter = 48 * time.Hour

// freshness grades one source by how far behind it has fallen.
func freshness(source string, newest *time.Time, now time.Time) assurance.SourceCoverage {
	if newest == nil {
		// Nothing there at all. A new installation with no mail yet is not a
		// broken connector, but it is equally not a source that confirmed
		// anything — and the readiness rule is right to hold back on both.
		return assurance.SourceCoverage{Source: source, State: assurance.CoverageUnavailable}
	}
	if now.Sub(*newest) > staleAfter {
		// Stale carries NO checked-through date. A date here would read as
		// "checked up to then", when what happened is that nothing arrived
		// since and we cannot tell a quiet week from a broken connector.
		return assurance.SourceCoverage{Source: source, State: assurance.CoverageStale}
	}
	return assurance.SourceCoverage{
		Source: source, State: assurance.CoverageChecked, CheckedThrough: newest,
	}
}
