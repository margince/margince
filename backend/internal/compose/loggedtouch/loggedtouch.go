// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package loggedtouch answers draftcore.LoggedTouchReader from the activities
// store, so a draft counts a note or past meeting beyond its timeline page.
package loggedtouch

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// Reader reads through the activities store's own gate. ConversationAsOf
// applies relstrength.HappenedByNowSQL, the SQL form of
// draftcore.IsLoggedTouch, so the one row it returns is one the draft counts.
type Reader struct{ Store *activities.Store }

// NewestLoggedTouch is the newest note or past meeting on the record.
func (r Reader) NewestLoggedTouch(ctx context.Context, on draftcore.LoggedOn, now time.Time) ([]crmcontracts.Activity, error) {
	limit := 1
	recordType, id := on.RecordType, on.ID
	rows, _, err := r.Store.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType:       &recordType,
		EntityID:         &id,
		Kinds:            []string{string(crmcontracts.ActivityKindNote), string(crmcontracts.ActivityKindMeeting)},
		Limit:            &limit,
		WrittenByHuman:   true,
		ConversationAsOf: &now,
		WithinProjectID:  on.ProjectID,
	})
	return rows, err
}
