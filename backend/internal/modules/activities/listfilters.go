// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// What the timeline may be narrowed by on the record seam: the contract's
// listActivities parameters, bound to the fields the HTTP door fills.
const (
	filterKind             = "kind"
	filterChannelProvider  = "channel_provider"
	filterEntityType       = "entity_type"
	filterEntityID         = "entity_id"
	filterAssigneeID       = "assignee_id"
	filterProjectID        = "project_id"
	filterThreadKey        = "thread_key"
	filterSourceActivityID = "source_activity_id"
	filterOccurredAfter    = "occurred_after"
	filterOccurredBefore   = "occurred_before"
	filterWaitingReply     = "waiting_reply"
	filterReminderDue      = "reminder_due"
	filterIncludeArchived  = "include_archived"
	filterSort             = "sort"
)

var activityListFilters = storekit.FilterSet[ListActivitiesInput]{
	filterKind:            storekit.FilterWord(func(in *ListActivitiesInput, v *string) { in.Kind = v }),
	filterChannelProvider: storekit.FilterWord(func(in *ListActivitiesInput, v *string) { in.ChannelProvider = v }),
	filterEntityType:      storekit.FilterWord(func(in *ListActivitiesInput, v *string) { in.EntityType = v }),
	// The polymorphic activity_link target, so the id stays untyped: the paired
	// entity_type names it.
	filterEntityID: func(in *ListActivitiesInput, value string) error {
		id, err := ids.Parse(value)
		if err != nil {
			return errors.New("takes a uuid, in the 8-4-4-4-12 hex form")
		}
		in.EntityID = &id
		return nil
	},
	filterAssigneeID:       storekit.FilterID(func(in *ListActivitiesInput, id *ids.UserID) { in.AssigneeID = id }),
	filterProjectID:        storekit.FilterID(func(in *ListActivitiesInput, id *ids.ProjectID) { in.WithinProjectID = id }),
	filterSourceActivityID: storekit.FilterID(func(in *ListActivitiesInput, id *ids.ActivityID) { in.SourceActivityID = id }),
	filterThreadKey:        storekit.FilterWord(func(in *ListActivitiesInput, v *string) { in.ThreadKey = v }),
	filterOccurredAfter:    instantFilter(func(in *ListActivitiesInput, at *time.Time) { in.OccurredAfter = at }),
	filterOccurredBefore:   instantFilter(func(in *ListActivitiesInput, at *time.Time) { in.OccurredBefore = at }),
	filterWaitingReply:     storekit.FilterFlag(func(in *ListActivitiesInput, v *bool) { in.WaitingReply = *v }),
	filterReminderDue:      storekit.FilterFlag(func(in *ListActivitiesInput, v *bool) { in.RemindersDue = *v }),
	filterIncludeArchived:  storekit.FilterFlag(func(in *ListActivitiesInput, v *bool) { in.IncludeArchived = *v }),
	filterSort:             storekit.FilterWord(func(in *ListActivitiesInput, v *string) { in.Sort = v }),
}

// instantFilter binds an RFC 3339 instant, the form the contract's date-time
// parameters take.
func instantFilter(set func(*ListActivitiesInput, *time.Time)) storekit.FilterBinding[ListActivitiesInput] {
	return func(in *ListActivitiesInput, value string) error {
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return errors.New("takes an RFC 3339 instant, such as 2026-10-15T09:00:00Z")
		}
		set(in, &at)
		return nil
	}
}

// ListFilters names what the timeline can be narrowed by on the seam.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	if t != datasource.EntityActivity {
		return nil
	}
	return activityListFilters.Names()
}
