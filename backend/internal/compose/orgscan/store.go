// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// The org_scan row: one per (reader, account), the carrier of the read in
// flight and the keeper of the last findings that settled.
//
// The reader predicate is written into every statement explicitly. Nothing
// narrows a query by workspace, so a statement that forgot user_id would
// serve one colleague's scan to another.
//
// Every transition announces itself on the AI activity rail from inside the
// transaction that made it. The row IS the occurrence: its id is the key,
// its status is the rail's own vocabulary, and its attempt count is what
// orders two events about one read.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The rail's vocabulary, which is also the column's CHECK.
const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusDegraded = "degraded"
	StatusFailed   = "failed"
)

const (
	// ActivitySource names this carrier to the rail projection. Identity, not
	// display: two sources must never collide on one occurrence key.
	ActivitySource = "account_scan"
	// ActivityKind is the display kind the rail's copy is keyed on. Exported
	// so the root gates hold it to the contract without importing this
	// package's internals.
	ActivityKind = "account_scan"
	// ActivityAITask is the api/ai-tasks.yaml task the read performs, exported
	// for the same gates.
	ActivityAITask = "account_scan"
	// ScanLease is how long a live occurrence stays believable. Past the
	// job's own timeout with a margin for the settle write, so the rail calls
	// a dead attempt stalled only once the worker would have given up on it.
	ScanLease = 5 * time.Minute
)

// row is one scan as stored.
type row struct {
	ID            ids.UUID
	UserID        ids.UserID
	OrgID         ids.OrganizationID
	Status        string
	Attempt       int
	RequestedAt   time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
	NextAttemptAt *time.Time
	Fingerprint   *string
	GeneratedAt   *time.Time
	GeneratedBy   *string
	DegradeReason *string
	ReadExchanges *int
	ReadDeals     *int
	Findings      []crmcontracts.Organization360Suggestion
}

func (r row) live() bool    { return r.Status == StatusQueued || r.Status == StatusRunning }
func (r row) settled() bool { return r.GeneratedAt != nil }

const rowColumns = `id, user_id, organization_id, status, attempt, requested_at, started_at, finished_at,
	next_attempt_at, fingerprint, generated_at, generated_by, degrade_reason, read_exchanges, read_deals, findings`

func scanRow(rows pgx.Row) (row, error) {
	var r row
	var findings []byte
	err := rows.Scan(&r.ID, &r.UserID, &r.OrgID, &r.Status, &r.Attempt, &r.RequestedAt, &r.StartedAt,
		&r.FinishedAt, &r.NextAttemptAt, &r.Fingerprint, &r.GeneratedAt, &r.GeneratedBy, &r.DegradeReason,
		&r.ReadExchanges, &r.ReadDeals, &findings)
	if err != nil {
		return row{}, err
	}
	if err := json.Unmarshal(findings, &r.Findings); err != nil {
		// Findings this build cannot read are findings it has not got: the
		// row still says where the read stands, and the next read rewrites
		// them.
		r.Findings = nil
	}
	return r, nil
}

// load is the reader's row for the account, or not found.
func load(ctx context.Context, tx pgx.Tx, userID ids.UserID, orgID ids.OrganizationID) (row, bool, error) {
	r, err := scanRow(tx.QueryRow(ctx,
		`SELECT `+rowColumns+` FROM org_scan WHERE user_id = $1 AND organization_id = $2`, userID, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, false, nil
	}
	if err != nil {
		return row{}, false, fmt.Errorf("orgscan: read the scan: %w", err)
	}
	return r, true, nil
}

// queue writes the reader's row as a read waiting to run — a fresh row, or
// the existing one re-armed with its last findings kept — and announces it.
// Re-arming counts a new attempt rather than starting the count over: the
// rail keys the row as one occurrence and takes only a later attempt's
// transitions, so an attempt that ran backwards would never be drawn.
func queue(ctx context.Context, tx pgx.Tx, userID ids.UserID, orgID ids.OrganizationID) (row, error) {
	r, err := scanRow(tx.QueryRow(ctx, `
		INSERT INTO org_scan (user_id, organization_id, status)
		VALUES ($1, $2, 'queued')
		ON CONFLICT (user_id, organization_id) DO UPDATE
		SET status = 'queued', attempt = org_scan.attempt + 1, requested_at = now(),
		    started_at = NULL, finished_at = NULL, next_attempt_at = NULL
		RETURNING `+rowColumns, userID, orgID))
	if err != nil {
		return row{}, fmt.Errorf("orgscan: queue the scan: %w", err)
	}
	return r, announce(ctx, tx, r)
}

// begin claims a queued read for this attempt. A row that is not queued —
// settled by a rival, or never queued — is not claimable, and the worker
// treats that as nothing to do rather than as a failure.
func begin(ctx context.Context, tx pgx.Tx, scanID ids.UUID) (row, bool, error) {
	r, err := scanRow(tx.QueryRow(ctx, `
		UPDATE org_scan
		   SET status = 'running', started_at = now(), next_attempt_at = NULL
		 WHERE id = $1 AND status = 'queued'
		RETURNING `+rowColumns, scanID))
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, false, nil
	}
	if err != nil {
		return row{}, false, fmt.Errorf("orgscan: claim the scan: %w", err)
	}
	return r, true, announce(ctx, tx, r)
}

// deferBudget parks a running read until the next budget window, as a new
// attempt of the same occurrence.
func deferBudget(ctx context.Context, tx pgx.Tx, scanID ids.UUID, next time.Time) error {
	r, err := scanRow(tx.QueryRow(ctx, `
		UPDATE org_scan
		   SET status = 'queued', next_attempt_at = $2, attempt = attempt + 1, started_at = NULL
		 WHERE id = $1 AND status = 'running'
		RETURNING `+rowColumns, scanID, next.UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("orgscan: defer the scan: %w", err)
	}
	return announce(ctx, tx, r)
}

// outcome is what a read that ran leaves on the row.
type outcome struct {
	Status        string
	Fingerprint   string
	GeneratedBy   crmcontracts.WrittenBy
	DegradeReason string
	ReadExchanges int
	ReadDeals     int
	Findings      []crmcontracts.Organization360Suggestion
}

// settle writes a finished read — done or degraded — and announces it.
func settle(ctx context.Context, tx pgx.Tx, scanID ids.UUID, out outcome) error {
	findings := out.Findings
	if findings == nil {
		findings = []crmcontracts.Organization360Suggestion{}
	}
	encoded, err := json.Marshal(findings)
	if err != nil {
		return fmt.Errorf("orgscan: encode the findings: %w", err)
	}
	r, err := scanRow(tx.QueryRow(ctx, `
		UPDATE org_scan
		   SET status = $2, finished_at = now(), generated_at = now(), fingerprint = $3,
		       generated_by = $4, degrade_reason = NULLIF($5, ''), read_exchanges = $6,
		       read_deals = $7, findings = $8, next_attempt_at = NULL
		 WHERE id = $1
		RETURNING `+rowColumns,
		scanID, out.Status, out.Fingerprint, string(out.GeneratedBy), out.DegradeReason,
		out.ReadExchanges, out.ReadDeals, encoded))
	if err != nil {
		return fmt.Errorf("orgscan: settle the scan: %w", err)
	}
	return announce(ctx, tx, r)
}

// fail closes a read that could not run, keeping whatever findings settled
// before it: the rules still answer, and the reason says what stopped the
// model.
func fail(ctx context.Context, tx pgx.Tx, scanID ids.UUID, reason string) error {
	r, err := scanRow(tx.QueryRow(ctx, `
		UPDATE org_scan
		   SET status = 'failed', finished_at = now(), degrade_reason = $2, next_attempt_at = NULL
		 WHERE id = $1
		RETURNING `+rowColumns, scanID, reason))
	if err != nil {
		return fmt.Errorf("orgscan: fail the scan: %w", err)
	}
	return announce(ctx, tx, r)
}

// subjectLabelBound is the contract's cap on the rail's subject label,
// applied before the wire.
const subjectLabelBound = 120

// announce publishes the row's state on the AI activity rail, inside the
// transaction that changed it.
//
// The ledger row comes first because the bus refuses an entity-less event
// without a trace link. The subject is the account, labelled with its name:
// the reader this occurrence belongs to is the one the account was already
// displayed to, which is the condition on carrying a label at all.
func announce(ctx context.Context, tx pgx.Tx, r row) error {
	ledgerID, err := storekit.LogSystem(ctx, tx, "ai_task.state_changed", map[string]any{
		"source": ActivitySource, "occurrence_key": r.ID.String(), "state": r.Status, "attempt": r.Attempt,
	})
	if err != nil {
		return fmt.Errorf("orgscan: log the scan's state change: %w", err)
	}
	var label *string
	if err := tx.QueryRow(ctx,
		`SELECT left(display_name, $2) FROM organization WHERE id = $1`, r.OrgID, subjectLabelBound,
	).Scan(&label); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("orgscan: read the account's name for the rail: %w", err)
	}
	// The instant THIS attempt was enqueued: a deferral re-queues under a new
	// attempt, and the rail ages a live row from here.
	queuedAt := r.RequestedAt
	if r.NextAttemptAt != nil {
		queuedAt = *r.NextAttemptAt
	}
	task := ActivityAITask
	subjectType := "organization"
	subjectID := openapi_types.UUID(r.OrgID.UUID)
	payload := crmcontracts.InternalEventAiTaskStateChanged{
		Source:        ActivitySource,
		OccurrenceKey: r.ID.String(),
		Kind:          ActivityKind,
		AiTask:        &task,
		Attempt:       r.Attempt,
		State:         r.Status,
		QueuedAt:      queuedAt,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
		LeaseSeconds:  lease(r),
		DegradeReason: r.DegradeReason,
		SubjectType:   &subjectType,
		SubjectId:     &subjectID,
		SubjectLabel:  label,
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("orgscan: publish the scan's state change: %w", err)
	}
	return nil
}

// lease is how long a live row stays believable, or none for a settled one
// — it is not claiming to work.
func lease(r row) *int {
	if !r.live() {
		return nil
	}
	seconds := int(ScanLease.Seconds())
	return &seconds
}
