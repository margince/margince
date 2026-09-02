// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The bounds a caller may put in front of a reader. A commitment is a line and
// a request for help is a paragraph, not a document.
const (
	labelBound = 500
	proseBound = 2000
	// planCap bounds one week's list. A plan is what a person means to do in
	// five days; past this it is a backlog, and a backlog belongs in tasks.
	planCap = 50
)

// The states a commitment moves through.
const (
	StateOpen    = "open"
	StateDone    = "done"
	StateMissed  = "missed"
	StateDropped = "dropped"
)

// WeekStartFunc answers which Monday a moment belongs to.
//
// Injected rather than computed here, because compose/weekly already owns that
// answer and a module may not import it. Two spellings of "which week" is how a
// plan comes to sit beside a review of a different one.
type WeekStartFunc func(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error)

// Teammates answers whether the caller may read a named person's plan.
//
// The lead's half of this module. Declared here as the one question it asks, so
// the edge is injected by compose rather than imported — identity owns the
// chart, and this module owns the plan.
type Teammates interface {
	SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error)
}

// Store owns the plan tables.
type Store struct {
	db        *database.DB
	weekStart WeekStartFunc
	teammates Teammates
}

// NewStore binds the store to db.
func NewStore(db *database.DB, weekStart WeekStartFunc, teammates Teammates) *Store {
	return &Store{db: db, weekStart: weekStart, teammates: teammates}
}

// Plan is one rep's week, as they meant it to go.
type Plan struct {
	ID             ids.UUID
	OwnerID        ids.UUID
	LocalWeekStart time.Time
	Status         string
	Version        int64
	Commitments    []Commitment
	// Outcome is what the week came to, stamped at close. Nil while the week
	// is open: there is no outcome yet, and a zero would claim one.
	Outcome *Outcome
}

// Commitment is one thing a rep said they would do.
type Commitment struct {
	ID    ids.UUID
	Label string
	// LinkedRecord is the record this is about, when it names one. Never
	// joined and never resolved here: the client asks the record's own
	// endpoint, so a record the reader may not see simply does not resolve.
	LinkedRecordType string
	LinkedRecordID   ids.UUID
	DueOn            *time.Time
	State            string
	HelpRequested    string
	ManagerResponse  string
	ManagerUserID    *ids.UUID
	RespondedAt      *time.Time
	CompletedAt      *time.Time
	Position         int
	Version          int64
}

// planUser is the acting rep.
//
// A plan is a personal record — whose week it was — so there is no argument by
// which a caller asks for their own. The lead's path is separate and states
// whose week it wants; see PlanFor.
func planUser(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// Current answers the caller's plan for the week containing now, or ErrNotFound
// when they have not started one.
//
// It never writes. A read that created a row would put an empty plan in every
// rep's history the first time they opened the page, and "did this person plan
// their week" would stop being answerable.
func (s *Store) Current(ctx context.Context, now time.Time) (Plan, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionRead); err != nil {
		return Plan{}, err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return Plan{}, err
	}
	return s.planForOwner(ctx, owner, now)
}

// PlanFor answers a named rep's current plan, for their lead.
//
// The SECOND reader, and the only one that names a person. Gated on the same
// teammate question attention.resolveOwner and notices.RaiseCoachNotice ask,
// through the same seam — never a fourth spelling of "is this my colleague".
//
// A reader who is not their lead gets ErrNotFound rather than a refusal: whether
// a person has a plan is itself something a stranger may not learn.
func (s *Store) PlanFor(ctx context.Context, owner ids.UUID, now time.Time) (Plan, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionRead); err != nil {
		return Plan{}, err
	}
	me, err := planUser(ctx)
	if err != nil {
		return Plan{}, err
	}
	if owner != me {
		if err := s.requireLeadOf(ctx, owner); err != nil {
			return Plan{}, err
		}
	}
	return s.planForOwner(ctx, owner, now)
}

// requireLeadOf refuses a caller who does not lead the named rep.
//
// Fails CLOSED when no membership reader is bound: a lead path with nothing
// answering the question must refuse, never admit. ErrNotFound throughout, so a
// stranger cannot tell an unbound seam from a colleague they do not lead.
func (s *Store) requireLeadOf(ctx context.Context, owner ids.UUID) error {
	if s.teammates == nil {
		return apperrors.ErrNotFound
	}
	// A lead reads a colleague's plan; own-scope reaches only their own, which
	// the caller-equals-owner case above already answered.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Permissions.RowScope == principal.RowScopeOwn {
		return apperrors.ErrNotFound
	}
	shares, err := s.teammates.SharesLiveTeamWithCaller(ctx, owner)
	if err != nil {
		return err
	}
	if !shares {
		return apperrors.ErrNotFound
	}
	return nil
}

// planForOwner reads one owner's plan for the week containing now.
func (s *Store) planForOwner(ctx context.Context, owner ids.UUID, now time.Time) (Plan, error) {
	var plan Plan
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		week, err := s.weekStart(ctx, tx, now)
		if err != nil {
			return err
		}
		plan, err = readPlan(ctx, tx, owner, week)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// readPlan reads one plan and its commitments, or reports there is none.
func readPlan(ctx context.Context, tx pgx.Tx, owner ids.UUID, week time.Time) (Plan, error) {
	plan := Plan{OwnerID: owner, LocalWeekStart: week}
	var due, kept *int
	err := tx.QueryRow(ctx, `
		SELECT id, local_week_start, status, version, commitments_due, commitments_kept
		  FROM weekly_plan
		 WHERE owner_id = $1 AND local_week_start = $2`, owner, week).
		Scan(&plan.ID, &plan.LocalWeekStart, &plan.Status, &plan.Version, &due, &kept)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Plan{}, fmt.Errorf("weeklyplan: reading the plan: %w", err)
	}
	if due != nil && kept != nil {
		plan.Outcome = &Outcome{Due: *due, Kept: *kept}
	}
	plan.Commitments, err = readCommitments(ctx, tx, plan.ID)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// readCommitments reads one plan's commitments in the order the rep put them in.
func readCommitments(ctx context.Context, tx pgx.Tx, planID ids.UUID) ([]Commitment, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, label, coalesce(linked_record_type, ''), linked_record_id,
		       due_on, state, help_requested, manager_response,
		       manager_user_id, responded_at, completed_at, position, version
		  FROM weekly_plan_commitment
		 WHERE plan_id = $1
		 ORDER BY position, created_at`, planID)
	if err != nil {
		return nil, fmt.Errorf("weeklyplan: reading the commitments: %w", err)
	}
	defer rows.Close()
	var out []Commitment
	for rows.Next() {
		var c Commitment
		var linkedID *ids.UUID
		if err := rows.Scan(&c.ID, &c.Label, &c.LinkedRecordType, &linkedID,
			&c.DueOn, &c.State, &c.HelpRequested, &c.ManagerResponse,
			&c.ManagerUserID, &c.RespondedAt, &c.CompletedAt, &c.Position, &c.Version); err != nil {
			return nil, err
		}
		if linkedID != nil {
			c.LinkedRecordID = *linkedID
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// bounded trims and checks one field a caller supplied.
func bounded(field, value string, limit int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		return "", &values.ParseError{
			Field: field, Code: "too_long",
			Message: fmt.Sprintf("%s is at most %d characters", field, limit),
		}
	}
	return trimmed, nil
}
