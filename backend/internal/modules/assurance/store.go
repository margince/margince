// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Store owns the assurance tables.
type Store struct {
	db *database.DB
}

// NewStore wires the store to its pool.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// Run states.
const (
	StatusRunning    = "running"
	StatusComplete   = "complete"
	StatusIncomplete = "incomplete"
)

// What a finished run entitles a reader to conclude.
//
// ReadinessChecksIncomplete is not a worse NeedsReview. One says the pipeline
// has problems; the other says we could not look. Telling a manager the first
// when the second is true is the mistake this vocabulary prevents.
const (
	ReadinessReady               = "ready"
	ReadinessReadyWithExceptions = "ready_with_exceptions"
	ReadinessNeedsReview         = "needs_review"
	ReadinessChecksIncomplete    = "checks_incomplete"
)

// Source coverage states.
const (
	CoverageChecked           = "checked"
	CoverageStale             = "stale"
	CoverageUnavailable       = "unavailable"
	CoveragePermissionLimited = "permission_limited"
	// CoverageNotConnected is a source the workspace never configured. Distinct
	// from unavailable: there is nothing to fix, only something to decide, and
	// the two route to different people.
	CoverageNotConnected = "not_connected"
)

// SourceCoverage is one source and how far the run reached into it.
type SourceCoverage struct {
	Source string
	State  string
	// CheckedThrough is set only for a source actually read. A date on an
	// unavailable source would read as "checked up to yesterday" when nothing
	// was checked at all.
	CheckedThrough *time.Time
}

// StartRun opens a pass. It always succeeds if the database is reachable.
//
// The worker NEVER refuses to start. Refusing would produce no run at all in
// exactly the case this pass exists to report — a broken connector — and the
// API would have nothing to show while the brief waited on it.
func (s *Store) StartRun(ctx context.Context, tx pgx.Tx, asOf time.Time) (ids.UUID, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return ids.Nil, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.Nil, err
	}
	versions, err := json.Marshal(ruleVersions())
	if err != nil {
		return ids.Nil, fmt.Errorf("assurance: recording the rule versions: %w", err)
	}
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO assurance_run (as_of, rule_versions, status, captured_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		asOf, versions, StatusRunning, capturedBy).Scan(&id); err != nil {
		return ids.Nil, fmt.Errorf("assurance: opening the run: %w", err)
	}
	return id, nil
}

// ruleVersions is which version of each rule this run used. A finding
// appearing or vanishing because a rule moved is not the business moving.
func ruleVersions() map[string]string {
	out := make(map[string]string, len(Rules()))
	for _, rule := range Rules() {
		out[rule.Type] = rule.Version
	}
	return out
}

// RecordCoverage says how far the run reached into one source.
func (s *Store) RecordCoverage(ctx context.Context, tx pgx.Tx, runID ids.UUID, in SourceCoverage) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	if in.State != CoverageChecked && in.CheckedThrough != nil {
		return fmt.Errorf("assurance: a %s source has no checked-through date: nothing was read", in.State)
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO assurance_source_coverage
		    (run_id, source, state, checked_through, captured_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (run_id, source) DO UPDATE
		SET state = EXCLUDED.state,
		    checked_through = EXCLUDED.checked_through,
		    updated_at = now()`,
		runID, in.Source, in.State, in.CheckedThrough, capturedBy); err != nil {
		return fmt.Errorf("assurance: recording %s coverage: %w", in.Source, err)
	}
	return nil
}

// UpsertException records a finding, or notes that an existing one was seen
// again.
//
// The DO UPDATE touches status and observed only WHERE the row is still open.
// Without that clause, somebody who resolves a finding with "the value is
// correct" has it reopened by tonight's scan — because the condition is still
// true, which is exactly what that answer means. They would resolve the same
// thing every morning until they stopped reading.
//
// last_seen_at updates unconditionally, because a resolved exception the scan
// still finds is worth knowing about even while it stays resolved.
func (s *Store) UpsertException(ctx context.Context, tx pgx.Tx, f Finding, owner string) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	claim, err := json.Marshal(f.Claim)
	if err != nil {
		return fmt.Errorf("assurance: recording the claim: %w", err)
	}
	observed, err := json.Marshal(f.Observed)
	if err != nil {
		return fmt.Errorf("assurance: recording the observation: %w", err)
	}
	subjectID, err := ids.Parse(f.SubjectID)
	if err != nil {
		return fmt.Errorf("assurance: finding names a subject id it cannot parse: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO assurance_exception
		    (logical_key, type, subject_kind, subject_id, claim, observed,
		     severity, affected_minor, currency, owner_id, captured_by)
		VALUES ($1, $2, 'deal', $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (logical_key) DO UPDATE
		SET last_seen_at = now(),
		    updated_at = now(),
		    -- Only while it is still somebody's problem. A resolved row keeps
		    -- the value it was resolved against, which is what lets a later
		    -- scan tell "still true" from "changed since you answered".
		    observed = CASE WHEN assurance_exception.status = 'open'
		                    THEN EXCLUDED.observed ELSE assurance_exception.observed END,
		    severity = CASE WHEN assurance_exception.status = 'open'
		                    THEN EXCLUDED.severity ELSE assurance_exception.severity END`,
		LogicalKey(f), f.Type, subjectID, claim, observed,
		f.Severity, f.AffectedMinor, nullIfEmpty(f.Currency),
		nullableID(owner), capturedBy); err != nil {
		return fmt.Errorf("assurance: recording the finding: %w", err)
	}
	return nil
}

// FinishRun closes a pass with what it found and what it could reach.
func (s *Store) FinishRun(
	ctx context.Context, tx pgx.Tx, runID ids.UUID,
	eligibleDeals, eligibleSignals int, status, readiness string,
) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_run
		SET eligible_deals = $2, eligible_signals = $3, status = $4,
		    readiness = $5, updated_at = now()
		WHERE id = $1 AND status = 'running'`,
		runID, eligibleDeals, eligibleSignals, status, readiness)
	if err != nil {
		return fmt.Errorf("assurance: closing the run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already closed, or never opened. Either way this call would be
		// rewriting a verdict somebody may have read.
		return apperrors.ErrNotFound
	}
	// The RUN is what the audit log records, not each finding. A pass over a
	// large pipeline mints thousands of exceptions, and an audit row per
	// finding would bury the one fact worth recovering later: that the
	// installation's inputs were checked on this night, by this version of the
	// rules, and what the pass concluded.
	//
	// Written at FINISH rather than at start, because a run's verdict is what
	// somebody acts on and a row recording only that a pass began says nothing
	// about what it found.
	auditID, err := storekit.AuditEvent(ctx, tx, "create", "assurance_run", runID,
		map[string]any{
			"status":           status,
			"readiness":        readiness,
			"eligible_deals":   eligibleDeals,
			"eligible_signals": eligibleSignals,
		})
	if err != nil {
		return err
	}
	// The Morning Brief waits on this: a brief assembled before the night's
	// check has nothing to say about whether the numbers in it were verified.
	//
	// The counts ride along and the findings do not. A subscriber acting on a
	// list that arrived on a bus is acting on a list somebody may already have
	// resolved; the counts say how much of the pipeline was covered, which is
	// the fact a consumer needs to decide whether to trust the run at all.
	event := crmcontracts.PublicEventForecastAssuranceCreated{
		RunId:           openapi_types.UUID(runID),
		Status:          crmcontracts.PublicEventForecastAssuranceCreatedStatus(status),
		EligibleDeals:   eligibleDeals,
		EligibleSignals: &eligibleSignals,
	}
	if readiness != "" {
		verdict := crmcontracts.PublicEventForecastAssuranceCreatedReadiness(readiness)
		event.Readiness = &verdict
	}
	return storekit.EmitEvent(ctx, tx, auditID, runID, event)
}

// LatestRun answers the most recent completed pass.
func (s *Store) LatestRun(ctx context.Context) (Run, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return Run{}, err
	}
	var out Run
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, as_of, eligible_deals, eligible_signals, status, readiness
			FROM assurance_run
			WHERE status <> 'running'
			ORDER BY as_of DESC, id DESC
			LIMIT 1`).
			Scan(&out.ID, &out.AsOf, &out.EligibleDeals, &out.EligibleSignals,
				&out.Status, &out.Readiness)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, apperrors.ErrNotFound
		}
		return Run{}, err
	}
	return out, nil
}

// Run is one completed pass, as a reader sees it.
type Run struct {
	ID              ids.UUID
	AsOf            time.Time
	EligibleDeals   int
	EligibleSignals int
	Status          string
	Readiness       *string
}

// InTx runs fn inside one workspace transaction.
//
// Gated on read even though it reads nothing itself: everything reachable
// through it begins with reading the forecast, and an ungated transaction
// opener is a door whose lock is somebody else's responsibility to remember.
func (s *Store) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return fn(ctx, tx)
	})
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableID(raw string) *ids.UUID {
	if raw == "" {
		return nil
	}
	parsed, err := ids.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

// CoverageFor reads which sources one run reached.
//
// Its own read rather than a join on the run, because a reader asking "is this
// number trustworthy" wants the coverage even when the run itself is stale —
// and a single row that carried both would make the absence of coverage look
// like the absence of a run.
func (s *Store) CoverageFor(ctx context.Context, runID ids.UUID) ([]SourceCoverage, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []SourceCoverage
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT source, state, checked_through
			FROM assurance_source_coverage
			WHERE run_id = $1
			ORDER BY source`, runID)
		if err != nil {
			return fmt.Errorf("assurance: reading the run's coverage: %w", err)
		}
		defer rows.Close()
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SourceCoverage, error) {
			var c SourceCoverage
			err := row.Scan(&c.Source, &c.State, &c.CheckedThrough)
			return c, err
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Exception is one finding as a reader meets it.
//
// Claim and Observed travel as raw JSON: they hold structured values whose
// shape is the exception type's, and decoding them here would make this struct
// the second place that knows what each type stores.
type Exception struct {
	ID          ids.UUID
	Type        string
	SubjectKind string
	SubjectID   ids.UUID
	Severity    string
	// AffectedMinor is how much money the finding puts in question, where that
	// can be said. Nil is different from zero: zero would claim nothing is at
	// stake.
	AffectedMinor *int64
	Currency      string
	OwnerID       *ids.UUID
	Status        string
	Claim         []byte
	Observed      []byte
	FirstSeenAt   time.Time
	LastSeenAt    time.Time
}
