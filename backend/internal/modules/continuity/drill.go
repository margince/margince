// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package continuity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Outcomes a drill can be recorded under.
const (
	OutcomeRunning = "running"
	OutcomePassed  = "passed"
	OutcomeFailed  = "failed"
)

// The two published recovery targets the ledger is read against: back within
// RecoveryTarget, and losing no more than DataLossTarget of data.
const (
	RecoveryTarget = 4 * time.Hour
	DataLossTarget = time.Hour
)

// Drill is one rehearsal of the restore procedure.
type Drill struct {
	ID         ids.UUID
	StartedAt  time.Time
	FinishedAt *time.Time
	RestoredTo time.Time
	Outcome    string
	Operator   string
	Notes      string
}

// RecoveryWindow is how long getting back took, and whether it is known yet.
// A drill still running has no answer, which is different from an answer of
// zero.
func (d Drill) RecoveryWindow() (time.Duration, bool) {
	if d.FinishedAt == nil {
		return 0, false
	}
	return d.FinishedAt.Sub(d.StartedAt), true
}

// DataLossWindow is how much time the restore point sat behind the failure it
// was standing in for — the figure the hourly claim is about.
func (d Drill) DataLossWindow() time.Duration {
	return d.StartedAt.Sub(d.RestoredTo)
}

// drillEntity names the ledger's rows in audit_log.
const drillEntity = "restore_drill"

// Store reads and writes the drill ledger.
type Store struct{ db *database.DB }

// NewStore binds the ledger to the installation's handle.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// Begin records that a drill has started, against the point in time it
// restored to, under the name of the operator running it.
//
// Written when it STARTS rather than when it finishes, so a drill that is
// abandoned halfway leaves a row saying so. A ledger written only on success
// cannot tell "we have never tried" from "we tried and it went badly", and
// those are the two readings somebody most needs to distinguish.
//
// The caller names the operator. A drill is recorded from the command line,
// where no session names anyone.
func (s *Store) Begin(ctx context.Context, restoredTo time.Time, operator, notes string) (ids.UUID, error) {
	if err := auth.Require(ctx, "installation_settings", principal.ActionUpdate); err != nil {
		return ids.Nil, err
	}
	operator = strings.TrimSpace(operator)
	if operator == "" {
		return ids.Nil, fmt.Errorf("continuity: a drill is recorded by somebody: %w", apperrors.ErrInvalidArgument)
	}
	var id ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var startedAt time.Time
		if err := tx.QueryRow(ctx, `
			INSERT INTO restore_drill (restored_to, operator, notes)
			VALUES ($1, $2, nullif($3, ''))
			RETURNING id, started_at`, restoredTo, operator, notes).Scan(&id, &startedAt); err != nil {
			return err
		}
		// The principal is the command line's system pass, so the typed
		// operator name travels as evidence beside it.
		_, err := storekit.AuditEventWithEvidence(ctx, tx, "create", drillEntity, id,
			map[string]any{"started_at": startedAt, "restored_to": restoredTo, "outcome": OutcomeRunning},
			map[string]any{"operator": operator})
		return err
	})
	if err != nil {
		return ids.Nil, fmt.Errorf("continuity: recording the start of a drill: %w", err)
	}
	return id, nil
}

// Finish closes a drill with what it proved.
//
// passed and failed are both recordable, and a drill already closed is not
// re-closable: the first answer is the one that happened, and letting a second
// overwrite it would turn the ledger into whatever its last writer preferred.
func (s *Store) Finish(ctx context.Context, id ids.UUID, outcome, notes string) error {
	if err := auth.Require(ctx, "installation_settings", principal.ActionUpdate); err != nil {
		return err
	}
	if outcome != OutcomePassed && outcome != OutcomeFailed {
		return fmt.Errorf("continuity: a drill closes as passed or failed, not %q: %w",
			outcome, apperrors.ErrInvalidArgument)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var finishedAt time.Time
		err := tx.QueryRow(ctx, `
			UPDATE restore_drill
			   SET finished_at = now(), outcome = $2,
			       notes = coalesce(nullif($3, ''), notes)
			 WHERE id = $1 AND outcome = 'running'
			 RETURNING finished_at`, id, outcome, notes).Scan(&finishedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("continuity: closing the drill: %w", err)
		}
		_, err = storekit.AuditEvent(ctx, tx, "close", drillEntity, id,
			map[string]any{"finished_at": finishedAt, "outcome": outcome})
		return err
	})
}

// Latest is the most recent drill, and whether there has ever been one. It asks
// `job_health:read`, the grant every System health report shares.
//
// The operator surface's whole question. "Never" is an answer it has to be
// able to give: an installation that has not rehearsed is the case the
// published numbers are least true of.
func (s *Store) Latest(ctx context.Context) (Drill, bool, error) {
	if err := auth.Require(ctx, "job_health", principal.ActionRead); err != nil {
		return Drill{}, false, err
	}
	var d Drill
	var notes *string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, started_at, finished_at, restored_to, outcome, operator, notes
			  FROM restore_drill
			 ORDER BY started_at DESC
			 LIMIT 1`).Scan(&d.ID, &d.StartedAt, &d.FinishedAt, &d.RestoredTo,
			&d.Outcome, &d.Operator, &notes)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Drill{}, false, nil
	}
	if err != nil {
		return Drill{}, false, fmt.Errorf("continuity: reading the last drill: %w", err)
	}
	if notes != nil {
		d.Notes = *notes
	}
	return d, true, nil
}
