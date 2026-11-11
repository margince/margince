// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// goalMetric is the figure a sales goal is set on: closed revenue, every pipeline.
const goalMetric crmcontracts.ReportingMetricID = "bookings_won"

// goalPlan is one seller's plan header for one calendar year.
type goalPlan struct {
	ID        ids.UUID  `json:"id"`
	OwnerID   ids.UUID  `json:"owner_id"`
	Year      int       `json:"year"`
	Currency  string    `json:"currency"`
	Version   int64     `json:"version"`
	Draft     *int64    `json:"draft_revision,omitempty"`
	Published *int64    `json:"published_revision,omitempty"`
	Withdrawn *int64    `json:"withdrawn_revision,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// goalTargetLink is one monthly target revision a publication wrote.
type goalTargetLink struct {
	Month    int      `json:"month"`
	TargetID ids.UUID `json:"target_id"`
	Revision int64    `json:"revision"`
}

// goalRevision is one immutable step of a plan's history.
type goalRevision struct {
	Revision   int64            `json:"revision"`
	Kind       string           `json:"kind"`
	Months     []*int64         `json:"months"`
	Declared   *int64           `json:"declared_total,omitempty"`
	Origin     string           `json:"origin"`
	Reason     string           `json:"reason"`
	Targets    []goalTargetLink `json:"targets,omitempty"`
	CreatedBy  ids.UUID         `json:"created_by"`
	AuthorName string           `json:"-"`
	CreatedAt  time.Time        `json:"created_at"`
}

const (
	goalKindDraft     = "draft"
	goalKindPublished = "published"
	goalKindDiscarded = "discarded"
	goalKindWithdrawn = "withdrawn"
)

// lockGoalYear serializes every write to one seller's goals for one year: plan
// drafts, publications and the target routes that would change those months.
// It holds whether or not a plan header exists yet.
func lockGoalYear(ctx context.Context, tx pgx.Tx, owner ids.UUID, year int) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "sales_goal:"+owner.String()+":"+strconv.Itoa(year))
	return err
}

const goalPlanColumns = "id,owner_id,year,currency,version,draft_revision,published_revision,withdrawn_revision,created_at,updated_at"

// loadGoalPlan reads the header, nil when the seller has no plan for the year.
func loadGoalPlan(ctx context.Context, tx pgx.Tx, owner ids.UUID, year int) (*goalPlan, error) {
	var p goalPlan
	err := tx.QueryRow(ctx, "SELECT "+goalPlanColumns+" FROM sales_goal_plan WHERE owner_id=$1 AND year=$2", owner, year).
		Scan(&p.ID, &p.OwnerID, &p.Year, &p.Currency, &p.Version, &p.Draft, &p.Published, &p.Withdrawn, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no plan is an answer, not a failure
	}
	return &p, err
}

func insertGoalPlan(ctx context.Context, tx pgx.Tx, p goalPlan) error {
	var b bindings
	values := []string{b.add(p.ID), b.add(p.OwnerID), b.add(p.Year), b.add(p.Currency), b.add(p.Version), b.add(p.CreatedAt), b.add(p.UpdatedAt)}
	_, err := tx.Exec(ctx, "INSERT INTO sales_goal_plan(id,owner_id,year,currency,version,created_at,updated_at) VALUES ("+strings.Join(values, ",")+")", b.values...)
	return err
}

func updateGoalPlan(ctx context.Context, tx pgx.Tx, p goalPlan) error {
	var b bindings
	query := "UPDATE sales_goal_plan SET version=" + b.add(p.Version) + ",draft_revision=" + b.add(p.Draft) +
		",published_revision=" + b.add(p.Published) + ",withdrawn_revision=" + b.add(p.Withdrawn) +
		",updated_at=" + b.add(p.UpdatedAt) + " WHERE id=" + b.add(p.ID)
	_, err := tx.Exec(ctx, query, b.values...)
	return err
}

func insertGoalRevision(ctx context.Context, tx pgx.Tx, plan ids.UUID, r goalRevision) error {
	months, err := json.Marshal(r.Months)
	if err != nil {
		return err
	}
	var targets []byte
	if r.Kind == goalKindPublished {
		if targets, err = json.Marshal(r.Targets); err != nil {
			return err
		}
	}
	var b bindings
	values := []string{b.add(plan), b.add(r.Revision), b.add(r.Kind), b.add(months), b.add(r.Declared), b.add(r.Origin), b.add(r.Reason), b.add(targets), b.add(r.CreatedBy), b.add(r.CreatedAt)}
	_, err = tx.Exec(ctx, "INSERT INTO sales_goal_plan_revision(plan_id,revision,kind,months,declared_total,origin,reason,targets,created_by,created_at) VALUES ("+strings.Join(values, ",")+")", b.values...)
	return err
}

// goalRevisions reads a plan's history in revision order, with each author's name.
func goalRevisions(ctx context.Context, tx pgx.Tx, plan ids.UUID) ([]goalRevision, error) {
	rows, err := tx.Query(ctx, `SELECT r.revision,r.kind,r.months,r.declared_total,r.origin,r.reason,r.targets,r.created_by,COALESCE(u.display_name,''),r.created_at
		FROM sales_goal_plan_revision r LEFT JOIN app_user u ON u.id=r.created_by WHERE r.plan_id=$1 ORDER BY r.revision`, plan)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (goalRevision, error) {
		var r goalRevision
		var months, targets []byte
		if err := row.Scan(&r.Revision, &r.Kind, &months, &r.Declared, &r.Origin, &r.Reason, &targets, &r.CreatedBy, &r.AuthorName, &r.CreatedAt); err != nil {
			return r, err
		}
		if err := json.Unmarshal(months, &r.Months); err != nil {
			return r, err
		}
		if targets != nil {
			return r, json.Unmarshal(targets, &r.Targets)
		}
		return r, nil
	})
}

// revisionAt finds one revision in a history read by goalRevisions.
func revisionAt(history []goalRevision, revision *int64) *goalRevision {
	if revision == nil {
		return nil
	}
	for index := range history {
		if history[index].Revision == *revision {
			return &history[index]
		}
	}
	return nil
}

// nextRevision is the number the next step of a plan's history takes.
func nextRevision(history []goalRevision) int64 {
	if len(history) == 0 {
		return 1
	}
	return history[len(history)-1].Revision + 1
}
