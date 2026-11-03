// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A decision recorded from a sales review is a task on its deal. It is
// written through the activities store, so it lives where every other task
// lives. decisionSourceSystem marks those tasks; nothing else writes it.
const decisionSourceSystem = provenance.AnalyticsDecisionSource

// decisionOutcomeMarker separates the expected result from the recorded one
// inside the task's body. The two are one text on the task, as a rep reads
// it, and are read back apart here.
const decisionOutcomeMarker = "\n\nOutcome: "

// decisionListLimit bounds the list a review reads: the latest decisions,
// not every task ever recorded. decisionMatchLimit is how far back a repeat
// of one decision on one deal is looked for.
const (
	decisionListLimit  = 50
	decisionMatchLimit = 500
)

// salesDecisions is the seam between the sales review and the activities
// store. It owns no table: a decision is a task, read back as a decision.
type salesDecisions struct {
	db          *database.DB
	activities  *activities.Store
	initiatives *initiatives.Store
}

func newSalesDecisions(db *database.DB) salesDecisions {
	return salesDecisions{db: db, activities: activities.NewStore(db), initiatives: newInitiativeStore(db.Pool())}
}

// decisionFilter narrows the decision list. Selection, when it names a scope
// or a dimension, keeps the decisions on the deals that review reads.
type decisionFilter struct {
	deal      *ids.UUID
	status    *string
	selection *crmcontracts.ReportingSelection
}

// Record writes the decision as a task on the deal, or returns the open
// task that already holds this decision on this deal. The second answer is
// what a retried confirmation and a decision reached from another filter
// both get.
func (s salesDecisions) Record(ctx context.Context, in crmcontracts.SalesDecisionInput) (crmcontracts.SalesDecision, bool, error) {
	decision := strings.TrimSpace(in.Decision)
	if decision == "" {
		return crmcontracts.SalesDecision{}, false, httperr.Validation("decision", "required", "say what was decided")
	}
	deal := ids.UUID(in.DealId)
	var out crmcontracts.SalesDecision
	var created bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		owner := ids.UserID{UUID: ids.UUID(in.OwnerId)}
		existing, err := s.openDecisionTx(ctx, tx, deal, owner.UUID, decision)
		if err != nil {
			return err
		}
		if existing != nil {
			out = *existing
			return nil
		}
		// The owner is handed work, so the caller's reach to them is asked
		// as it is for any hand-over.
		if err := auth.EnsureAssignee(ctx, tx, owner.UUID); err != nil {
			return err
		}
		due, err := reviewDue(ctx, tx, in.ReviewOn.Time)
		if err != nil {
			return err
		}
		kind := "task"
		source := decisionSourceSystem
		sourceID := ids.NewV7().String()
		var body *string
		if in.ExpectedResult != nil && strings.TrimSpace(*in.ExpectedResult) != "" {
			expected := strings.TrimSpace(*in.ExpectedResult)
			body = &expected
		}
		task, _, err := s.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
			Kind:             kind,
			Subject:          &decision,
			Body:             body,
			DueAt:            &due,
			AssigneeID:       &owner,
			SourceSystem:     &source,
			SourceID:         &sourceID,
			Links:            []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
			LinksFromRequest: true,
		})
		if err != nil {
			return err
		}
		out, created = decisionOf(task), true
		return nil
	})
	return out, created, err
}

// Outcome records what work was completed and closes the task in one write.
// The deal is not touched: completed work is evidence of action, not of a
// sale.
func (s salesDecisions) Outcome(ctx context.Context, id ids.UUID, result string) (crmcontracts.SalesDecision, error) {
	result = strings.TrimSpace(result)
	if result == "" {
		return crmcontracts.SalesDecision{}, httperr.Validation("result", "required", "say what work was completed")
	}
	current, err := s.activities.GetActivity(ctx, ids.ActivityID{UUID: id}, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.SalesDecision{}, err
	}
	if current.SourceSystem == nil || *current.SourceSystem != decisionSourceSystem {
		return crmcontracts.SalesDecision{}, fmt.Errorf("the task is not a recorded decision: %w", httperr.Validation("id", "invalid", "this task is not a recorded decision"))
	}
	expected := ""
	if current.Body != nil {
		var recorded *string
		expected, recorded = splitOutcome(*current.Body)
		if recorded != nil {
			return crmcontracts.SalesDecision{}, fmt.Errorf("the outcome is recorded: %w", apperrors.ErrConflict)
		}
	}
	body := expected + decisionOutcomeMarker + result
	done := true
	updated, err := s.activities.UpdateActivity(ctx, ids.ActivityID{UUID: id}, activities.UpdateActivityInput{Body: &body, IsDone: &done})
	if err != nil {
		return crmcontracts.SalesDecision{}, err
	}
	return decisionOf(updated), nil
}

// List answers the decisions the filter keeps, newest first.
func (s salesDecisions) List(ctx context.Context, filter decisionFilter) ([]crmcontracts.SalesDecision, error) {
	kind, source, limit := "task", decisionSourceSystem, decisionListLimit
	in := activities.ListActivitiesInput{Kind: &kind, SourceSystem: &source, Limit: &limit}
	if filter.deal != nil {
		in.LinkedToDeals = []ids.UUID{*filter.deal}
	}
	if filter.selection != nil && reportingSelectionNarrows(*filter.selection) {
		reviewed, err := s.reviewedDeals(ctx, *filter.selection, in.LinkedToDeals)
		if err != nil {
			return nil, err
		}
		// No deal in the review is no decision, answered without a query. A
		// nil link list would read as no narrowing at all.
		if len(reviewed) == 0 {
			return []crmcontracts.SalesDecision{}, nil
		}
		in.LinkedToDeals = reviewed
	}
	tasks, _, err := s.activities.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.SalesDecision, 0, len(tasks))
	for _, task := range tasks {
		decision := decisionOf(task)
		if filter.status != nil && string(decision.Status) != *filter.status {
			continue
		}
		out = append(out, decision)
	}
	return out, nil
}

// reviewedDeals is the selection's deals, kept to the named ones when the
// caller names any.
func (s salesDecisions) reviewedDeals(ctx context.Context, selection crmcontracts.ReportingSelection, named []ids.UUID) ([]ids.UUID, error) {
	var deals []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		deals, err = reportingSelectionDeals(ctx, tx, s.initiatives, selection)
		return err
	})
	if err != nil || len(named) == 0 {
		return deals, err
	}
	kept := make([]ids.UUID, 0, len(named))
	for _, deal := range named {
		if slices.Contains(deals, deal) {
			kept = append(kept, deal)
		}
	}
	return kept, nil
}

// openDecisionTx finds the open task that already records this decision on
// this deal for this owner. A completed one, or the same decision handed to
// someone else, never blocks a new occurrence.
func (s salesDecisions) openDecisionTx(ctx context.Context, tx pgx.Tx, deal, owner ids.UUID, decision string) (*crmcontracts.SalesDecision, error) {
	kind, source, limit := "task", decisionSourceSystem, decisionMatchLimit
	tasks, _, err := s.activities.ListActivitiesIn(ctx, tx, activities.ListActivitiesInput{Kind: &kind, SourceSystem: &source, LinkedToDeals: []ids.UUID{deal}, Limit: &limit})
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if isDone(task) || task.Subject == nil || strings.TrimSpace(*task.Subject) != decision {
			continue
		}
		if task.AssigneeId == nil || ids.UUID(*task.AssigneeId) != owner {
			continue
		}
		found := decisionOf(task)
		return &found, nil
	}
	return nil, nil
}

// decisionOf reads a task back as the decision it records.
func decisionOf(task crmcontracts.Activity) crmcontracts.SalesDecision {
	out := crmcontracts.SalesDecision{Id: task.Id, Status: crmcontracts.SalesDecisionStatusOpen, CreatedAt: task.CreatedAt, OwnerId: task.AssigneeId, DoneAt: task.DoneAt}
	if task.Subject != nil {
		out.Decision = *task.Subject
	}
	if task.Links != nil {
		for _, link := range *task.Links {
			if link.EntityType == "deal" {
				out.DealId = link.EntityId
				break
			}
		}
	}
	if task.DueAt != nil {
		out.ReviewOn = &openapi_types.Date{Time: task.DueAt.UTC()}
	}
	if task.Body != nil {
		expected, result := splitOutcome(*task.Body)
		if expected != "" {
			out.ExpectedResult = &expected
		}
		if result != nil {
			out.Result = result
		}
	}
	if isDone(task) {
		out.Status = crmcontracts.SalesDecisionStatusDone
	}
	return out
}

func isDone(task crmcontracts.Activity) bool {
	return task.IsDone != nil && *task.IsDone
}

// reviewDue is the end of the review day in the installation's calendar, as
// a task picked for a day falls due.
func reviewDue(ctx context.Context, tx pgx.Tx, day time.Time) (time.Time, error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("the installation's time zone %q: %w", zone, err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, loc), nil
}

// splitOutcome reads the expected result and, after the marker, the
// recorded one.
func splitOutcome(body string) (expected string, result *string) {
	before, after, found := strings.Cut(body, decisionOutcomeMarker)
	if !found {
		return body, nil
	}
	return before, &after
}
