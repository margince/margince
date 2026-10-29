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

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// assuranceSubjectsSQL is AssuranceSubjects' one read, held at package
// level so the function stays within the length a reader holds at once.
const assuranceSubjectsSQL = `
		SELECT d.id, d.owner_id, d.amount_minor, d.currency,
		       d.expected_close_date, d.close_date_provisional,
		       d.forecast_category, s.name,
		       d.created_at, d.wait_until,
		       (SELECT max(a.occurred_at)
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.deal_id = d.id
		           AND a.archived_at IS NULL
		           AND %[2]s) AS last_customer_progress_at,
		       (SELECT min(a.occurred_at)
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.deal_id = d.id
		           AND a.archived_at IS NULL
		           AND a.kind = 'meeting'
		           AND %[3]s
		           AND EXISTS (SELECT 1 FROM activity_participant p
		                        WHERE p.activity_id = a.id AND p.contact_id IS NOT NULL
		                          AND p.response = 'accepted')) AS confirmed_meeting_at,
		       EXISTS (SELECT 1 FROM assurance_exception x
		                WHERE x.subject_kind = 'deal' AND x.subject_id = d.id
		                  AND x.type = $4 AND x.status = 'open') AS open_review,
		       %[4]s AS viability_fingerprint,
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
		       EXISTS (SELECT 1
		          FROM activity a
		          JOIN activity_link al ON al.activity_id = a.id
		         WHERE al.deal_id = d.id
		           AND a.archived_at IS NULL
		           AND ((a.kind = 'task' AND a.is_done = false AND %[1]s)
		                -- MeetingAheadSQL admits NULL, a meeting nothing has said
		                -- is off. An imported one carries no status, and the strict form raised a
		                -- "no next step" finding against a deal whose next step
		                -- was in the rep's calendar.
		                OR (a.kind = 'meeting' AND %[3]s))) AS has_next_step,
		       EXISTS (SELECT 1
		          FROM relationship r
		         WHERE r.kind = 'deal_stakeholder'
		           AND r.deal_id = d.id
		           AND r.role = $3
		           AND r.archived_at IS NULL) AS has_economic_buyer,
		       (SELECT count(*)
		          FROM stage_exit_criterion c
		         WHERE c.stage_id = d.stage_id
		           AND c.required
		           AND c.archived_at IS NULL
		           AND NOT EXISTS (SELECT 1
		              FROM deal_stage_evidence e
		             WHERE e.criterion_id = c.id
		               AND e.deal_id = d.id
		               AND e.met
		               AND e.refuted_at IS NULL
		               AND %[5]s)) AS unmet_exit_criteria
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE d.archived_at IS NULL AND d.status = 'open'
		ORDER BY d.id`

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
func AssuranceSubjects(
	ctx context.Context, tx pgx.Tx, asOf time.Time, coverage []assurance.SourceCoverage,
) ([]assurance.Subject, error) {
	now := asOf.UTC()
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	args := []any{now.Add(-closePushWindow), now, dealrole.EconomicBuyer, assurance.TypeDealViability}
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(assuranceSubjectsSQL, activities.CurrentTaskClause("a", now, identities, arg),
		customerProgressSQL("a", "$2"), relstrength.MeetingAheadSQL("a", "$2"),
		viabilityFingerprintSQL("d"), deals.SettlingEvidenceSQL("c", "e"))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals to check: %w", err)
	}
	defer rows.Close()

	mailCovered := sourceChecked(coverage, sourceMail)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.Subject, error) {
		return scanAssuranceSubject(row, now, zone, mailCovered)
	})
}

func scanAssuranceSubject(row pgx.CollectableRow, asOf time.Time, zone *time.Location, mailCovered bool) (assurance.Subject, error) {
	var out assurance.Subject
	var dealID ids.UUID
	var owner *ids.UUID
	var currency, category, stage *string
	var evidence deals.ViabilityInput
	var fingerprint string
	err := row.Scan(&dealID, &owner, &out.AmountMinor, &currency,
		&out.ExpectedClose, &out.CloseProvisional, &category, &stage,
		&evidence.CreatedAt, &evidence.CustomerWaitUntil, &evidence.LastCustomerProgressAt,
		&evidence.ConfirmedMeetingAt, &evidence.OpenReview, &fingerprint,
		&out.OfferTotalMinor, &out.ContractTotalMinor, &out.CloseDatePushes,
		&out.HasNextStep, &out.HasEconomicBuyer, &out.UnmetExitCriteria)
	if err != nil {
		return out, err
	}
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
	evidence.ExpectedClose, evidence.CloseProvisional = out.ExpectedClose, out.CloseProvisional
	evidence.HasNextStep, evidence.MailCovered = out.HasNextStep, mailCovered
	assessed := deals.AssessViability(evidence, asOf, zone)
	out.Viability = &assurance.SubjectViability{
		Outcome: assessed.Outcome, Reasons: assessed.Reasons, SilentDays: assessed.SilentDays,
		NextChange: assessed.NextChange, Fingerprint: fingerprint,
	}
	return out, nil
}

// customerProgressSQL admits a row that is the CUSTOMER moving the deal: mail
// they wrote, or a meeting with them that took place. Our own mail, a task, a
// field edit, a canceled meeting and a machine's mail never do. Mail from a
// colleague is outbound, so internal correspondence is not progress either.
func customerProgressSQL(alias, asOf string) string {
	return "(" + relstrength.DirectionClockSQL(alias) + " AND ((" + alias + ".direction = 'inbound' AND " +
		occurredday.BegunSQL(alias, asOf) + ") OR (" + alias + ".kind = 'meeting' AND " +
		activities.CustomerMeetingSQL(alias) + " AND " + occurredday.MeetingTookPlaceSQL(alias, asOf) + ")))"
}

// sourceChecked reports whether the run read one source on time.
func sourceChecked(coverage []assurance.SourceCoverage, source string) bool {
	for _, c := range coverage {
		if c.Source == source {
			return c.State == assurance.CoverageChecked
		}
	}
	return false
}

// AssuranceConfig is the check's configuration, with a receipt lifetime taken
// from the job catalog: one cadence plus one run's deadline. A receipt older
// than that has outlived the check that should have replaced it.
func AssuranceConfig() assurance.Config {
	cfg := assurance.DefaultConfig()
	if spec, ok := jobs.SpecFor(AssuranceSweepArgs{}.Kind()); ok {
		cfg.ReceiptLifetime = spec.Cadence.Fixed + spec.Timeout.Duration()
	}
	return cfg
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

// sourceMail names the mailbox in coverage rows; the assurance store's CHECK
// constraint owns the vocabulary.
const sourceMail = "mail"

// mailProviders are the capture connectors that carry mail. gcal is calendar
// and the chat providers are messaging; a healthy WhatsApp says nothing about
// whether anybody read the mailbox a close date would be checked against.
var mailProviders = []string{providerGmail, "imap", providerGraph, "offline_demo"}

// mailCoverage grades the mailbox by CONNECTOR health, not by mail volume.
//
// It used to read max(occurred_at) over captured email, which conflates two
// different facts: a quiet mailbox on a healthy connector was reported stale —
// nothing arrived, so "the source fell behind" — while a broken connector under
// recent mail read as checked. The sync checkpoint is the fact this state
// actually claims: when the connector last successfully read the source.
func mailCoverage(ctx context.Context, tx pgx.Tx, now time.Time) assurance.SourceCoverage {
	// Aggregates fail CLOSED over every live, not-deliberately-disconnected
	// connection: min() ignores NULL, so a never-synced broken mailbox next to
	// a healthy one would otherwise vanish from the aggregate and the run
	// would claim checked over a source it never read.
	var live, reauth, unsynced int
	var oldestSuccess *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE c.status <> 'disconnected'),
		       count(*) FILTER (WHERE c.status = 'reauth_required'),
		       count(*) FILTER (WHERE c.status <> 'disconnected' AND s.last_success_at IS NULL),
		       min(s.last_success_at) FILTER (WHERE c.status <> 'disconnected')
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.provider = ANY($1) AND c.archived_at IS NULL`,
		mailProviders).Scan(&live, &reauth, &unsynced, &oldestSuccess); err != nil {
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageUnavailable}
	}
	switch {
	case live == 0:
		// Never configured, or every mailbox deliberately disconnected. Either
		// way there is nothing to fix, only something to decide, and the two
		// route to different contacts than a broken connector does.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageNotConnected}
	case reauth > 0:
		// Somebody must re-grant access; until then part of the mailbox is
		// unreadable for a permission reason, not a technical one.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoveragePermissionLimited}
	case unsynced > 0:
		// At least one live mailbox has never been read. One healthy sibling
		// does not make the source checked — the claim covers all of them.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageUnavailable}
	case now.Sub(*oldestSuccess) > staleAfter:
		// The slowest mailbox decides: "mail was checked" over three mailboxes
		// means all three, so the oldest checkpoint grades the source and caps
		// the checked-through date below.
		return assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageStale}
	default:
		return assurance.SourceCoverage{
			Source: sourceMail, State: assurance.CoverageChecked, CheckedThrough: oldestSuccess,
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
