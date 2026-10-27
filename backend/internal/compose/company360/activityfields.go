// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// WithActivityFields lets the timeline section carry the activities' custom
// field values. Without it the section's rows carry none.
func (s *Service) WithActivityFields(store *activities.Store) *Service {
	s.activities = store
	return s
}

// activityColumns reads the timeline's custom-field columns above the page's
// transaction, because the catalog opens a connection of its own.
func (s *Service) activityColumns(ctx context.Context) (activities.CustomColumns, error) {
	if s.activities == nil {
		return activities.CustomColumns{}, nil
	}
	return s.activities.ActiveActivityColumns(ctx)
}

// readTimeline reads the first page of the account's timeline through the
// activities module's own list, the one GET /activities reads too. Its gate is that list's own
// auth.Require, which refuses with the same error every other section uses
// to declare itself omitted.
func (a *assembly) readTimeline() error {
	companyUUID := a.companyID.UUID
	entityType := "company"
	limit := sectionLimit
	var conversationAsOf *time.Time
	if a.opts.WrittenByHumanOnly {
		conversationAsOf = &a.now
	}
	data, page, err := activities.ListActivitiesTx(a.ctx, a.tx, activities.ListActivitiesInput{
		EntityType:       &entityType,
		EntityID:         &companyUUID,
		Limit:            &limit,
		WithinProjectID:  a.opts.ProjectID,
		WrittenByHuman:   a.opts.WrittenByHumanOnly,
		ConversationAsOf: conversationAsOf,
		TaskAsOf:         &a.now,
		Columns:          a.activityColumns,
	})
	if err != nil {
		return err
	}
	a.out.Activities = &crmcontracts.ActivityListResponse{Data: data, Page: pageInfo(page)}
	return nil
}
