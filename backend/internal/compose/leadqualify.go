// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The reminder for a lead worked from an existing contact that nobody has
// qualified or closed. The contacts module knows which leads are due; the
// reminder is a task, which is an activity, so the edge sits here and rides
// the clock-trigger pass beside the first-response scan.
//
// The task is a system task linked to the lead, so the auto-resolve
// workflows in activities/followupresolve.go close it when the lead is
// qualified or disqualified, or when somebody logs a call, mail or meeting on
// it: each of those is the follow-up the reminder asks for.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadQualifyReminderAge is how long a lead worked from a contact may stay
// open before its owner is reminded to qualify it or close it.
const leadQualifyReminderAge = 14 * 24 * time.Hour

// leadQualifyReminderSubject is the reminder task's title.
const leadQualifyReminderSubject = "Qualify this lead or close it"

// remindUnqualifiedLeads logs one reminder task per lead due one, assigned to
// the lead's owner, or unassigned for an ownerless lead so it lands in the
// unassigned queue.
func remindUnqualifiedLeads(ctx context.Context, db *database.DB, now func() time.Time, log *slog.Logger) error {
	wsCtx := principal.SystemActing(ctx, "system:lead-qualify-scan")
	at := now().UTC()
	due, err := contacts.NewStore(db).LeadsDueQualifyReminder(wsCtx, at.Add(-leadQualifyReminderAge))
	if err != nil {
		return fmt.Errorf("lead qualify reminder scan: %w", err)
	}
	store := activities.NewStore(db)
	var failed error
	for _, lead := range due {
		subject := leadQualifyReminderSubject
		sourceSystem := contacts.QualifyReminderSource
		sourceID := lead.LeadID.String()
		in := activities.LogActivityInput{
			Kind:         activityKindTask,
			Subject:      &subject,
			OccurredAt:   &at,
			DueAt:        &at,
			SourceSystem: &sourceSystem,
			SourceID:     &sourceID,
			Links:        []activities.ActivityLinkInput{{EntityType: entityLead, EntityID: lead.LeadID.UUID}},
			Source:       systemActor,
			AssigneeID:   lead.OwnerID,
		}
		if err := logQualifyReminder(wsCtx, store, in); err != nil {
			// One lead's failure does not hold back the others: each task is
			// its own write, and the next pass retries the ones that failed.
			failed = errors.Join(failed, fmt.Errorf("reminding about lead %s: %w", lead.LeadID, err))
		}
	}
	if len(due) > 0 {
		log.InfoContext(wsCtx, "lead qualify reminders logged", "count", len(due))
	}
	return failed
}

// logQualifyReminder writes one reminder. An owner who can no longer hold work
// (deactivated, archived, an agent) costs the task its assignee, not its
// existence, as unassignableSeat rules for the assurance bundle.
func logQualifyReminder(ctx context.Context, store *activities.Store, in activities.LogActivityInput) error {
	_, _, err := store.LogActivity(ctx, in)
	if err == nil || in.AssigneeID == nil || !unassignableSeat(err) {
		return err
	}
	in.AssigneeID = nil
	_, _, err = store.LogActivity(ctx, in)
	return err
}
